package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRequireEmptyInitializationTargetRejectsExistingSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/init.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, requireEmptyInitializationTarget(db))
	require.NoError(t, db.Exec("CREATE TABLE existing_data (id INTEGER PRIMARY KEY)").Error)
	require.ErrorContains(t, requireEmptyInitializationTarget(db), "production migration runner")
}
