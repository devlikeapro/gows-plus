package sqlstorage

import (
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// The pool settings are plain database/sql, so any driver exercises them
func TestConfigurePsqlKeepsConnectionsIdle(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, configurePsql(db))

	// Hand one connection back to the pool
	_, err = db.Exec("SELECT 1")
	require.NoError(t, err)
	require.Equal(t, 1, db.Stats().Idle)

	// database/sql's cleaner ticks at most once per second, so a sub-second
	// idle timeout (a bare 30 is 30ns) reaps the connection right here
	time.Sleep(1500 * time.Millisecond)

	require.Equal(t, 1, db.Stats().Idle, "idle connection was reaped before it could be reused")
}

// Guards the unit of the argument: database/sql has no getter for the idle
// timeout, so a bare 30 (30ns) is indistinguishable from 30s without reflection
func TestConfigurePsqlIdleTimeoutIsThirtySeconds(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, configurePsql(db))

	field := reflect.ValueOf(db.DB).Elem().FieldByName("maxIdleTime")
	require.True(t, field.IsValid(), "database/sql renamed maxIdleTime, this guard needs updating")

	require.Equal(t, 30*time.Second, time.Duration(field.Int()))
}
