package dataprofile

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func prepareSanitizedTestDatabase(t *testing.T, sourcePath string) string {
	t.Helper()
	preparedPath := filepath.Join(t.TempDir(), "prepared.db")
	require.NoError(t, copyRegularFile(sourcePath, preparedPath, 0o600))
	db, err := sql.Open("sqlite3", "file:"+preparedPath+"?_foreign_keys=on&_busy_timeout=5000")
	require.NoError(t, err)
	require.NoError(t, SanitizeSnapshot(db))
	require.NoError(t, db.Close())
	return preparedPath
}

func publishPreparedTestSnapshot(
	t *testing.T,
	profileHome string,
	id string,
	sourcePath string,
	capturedAt string,
	sanitizerVersion string,
) (Snapshot, error) {
	t.Helper()
	preparedPath := sourcePath
	if sanitizerVersion == SanitizerVersion {
		preparedPath = prepareSanitizedTestDatabase(t, sourcePath)
	}
	return PublishPreparedSnapshot(
		profileHome,
		id,
		preparedPath,
		capturedAt,
		sanitizerVersion,
	)
}
