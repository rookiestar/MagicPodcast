package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const logicalFingerprintVersion = "sqlite-logical-v1"

type fingerprintColumn struct {
	cid      int
	name     string
	typeName string
	pk       int
}

// logicalDatabaseFingerprint binds a backup to the exact database state that
// was previewed. It hashes schema objects and every user-table value in a
// deterministic order without exposing any value in the command output.
func logicalDatabaseFingerprint(ctx context.Context, db *gorm.DB) (string, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	writeFrame := func(kind string, value []byte) {
		hash.Write([]byte(kind))
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		hash.Write(length[:])
		hash.Write(value)
	}
	writeFrame("fingerprint-version", []byte(logicalFingerprintVersion))

	rows, err := sqlDB.QueryContext(ctx, `
		SELECT type, name, COALESCE(tbl_name, ''), COALESCE(sql, '')
		FROM sqlite_master
		WHERE type IN ('table', 'index', 'trigger', 'view')
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name
	`)
	if err != nil {
		return "", err
	}
	var tables []string
	for rows.Next() {
		var objectType, name, tableName, definition string
		if err := rows.Scan(&objectType, &name, &tableName, &definition); err != nil {
			rows.Close()
			return "", err
		}
		writeFrame("schema-type", []byte(objectType))
		writeFrame("schema-name", []byte(name))
		writeFrame("schema-table", []byte(tableName))
		writeFrame("schema-sql", []byte(definition))
		if objectType == "table" {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}

	for _, table := range tables {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		columns, err := fingerprintTableColumns(ctx, sqlDB, table)
		if err != nil {
			return "", err
		}
		writeFrame("table-name", []byte(table))
		for _, column := range columns {
			writeFrame("column-cid", []byte(strconv.Itoa(column.cid)))
			writeFrame("column-name", []byte(column.name))
			writeFrame("column-type", []byte(column.typeName))
			writeFrame("column-pk", []byte(strconv.Itoa(column.pk)))
		}
		query := "SELECT * FROM " + quoteSQLiteIdentifier(table)
		orderColumns := append([]fingerprintColumn(nil), columns...)
		// Primary-key order is stable across copies. Tables without a primary key
		// use all columns; duplicate rows are indistinguishable in the hash.
		sortFingerprintColumns(orderColumns)
		if len(orderColumns) > 0 {
			parts := make([]string, 0, len(orderColumns))
			for _, column := range orderColumns {
				parts = append(parts, quoteSQLiteIdentifier(column.name))
			}
			query += " ORDER BY " + strings.Join(parts, ", ")
		}
		dataRows, err := sqlDB.QueryContext(ctx, query)
		if err != nil {
			return "", err
		}
		valueColumns, err := dataRows.Columns()
		if err != nil {
			dataRows.Close()
			return "", err
		}
		for dataRows.Next() {
			values := make([]any, len(valueColumns))
			pointers := make([]any, len(values))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err := dataRows.Scan(pointers...); err != nil {
				dataRows.Close()
				return "", err
			}
			writeFrame("row", []byte(strconv.Itoa(len(values))))
			for _, value := range values {
				writeFingerprintValue(writeFrame, value)
			}
		}
		if err := dataRows.Err(); err != nil {
			dataRows.Close()
			return "", err
		}
		if err := dataRows.Close(); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fingerprintTableColumns(ctx context.Context, db *sql.DB, table string) ([]fingerprintColumn, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteSQLiteIdentifier(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []fingerprintColumn
	for rows.Next() {
		var column fingerprintColumn
		var notNull int
		var defaultValue sql.NullString
		if err := rows.Scan(&column.cid, &column.name, &column.typeName, &notNull, &defaultValue, &column.pk); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func sortFingerprintColumns(columns []fingerprintColumn) {
	for i := 1; i < len(columns); i++ {
		current := columns[i]
		j := i - 1
		for j >= 0 && fingerprintColumnBefore(current, columns[j]) {
			columns[j+1] = columns[j]
			j--
		}
		columns[j+1] = current
	}
}

func fingerprintColumnBefore(left, right fingerprintColumn) bool {
	if left.pk > 0 && right.pk == 0 {
		return true
	}
	if left.pk == 0 && right.pk > 0 {
		return false
	}
	if left.pk > 0 && right.pk > 0 && left.pk != right.pk {
		return left.pk < right.pk
	}
	return left.cid < right.cid
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func writeFingerprintValue(writeFrame func(string, []byte), value any) {
	switch typed := value.(type) {
	case nil:
		writeFrame("value-null", nil)
	case int64:
		writeFrame("value-int64", []byte(strconv.FormatInt(typed, 10)))
	case float64:
		writeFrame("value-float64", []byte(strconv.FormatFloat(typed, 'g', -1, 64)))
	case bool:
		writeFrame("value-bool", []byte(strconv.FormatBool(typed)))
	case []byte:
		writeFrame("value-blob", typed)
	case string:
		writeFrame("value-text", []byte(typed))
	default:
		writeFrame("value-other", []byte(fmt.Sprintf("%T:%v", typed, typed)))
	}
}
