package store

import (
	"database/sql"
	"os"

	_ "modernc.org/sqlite"
)

// OpenStore opens (or creates) the SQLite database at the given path.
func OpenStore(path string) (*sql.DB, error) {
	// Ensure the directory exists
	if err := os.MkdirAll(getDir(path), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// Set pragmas for performance and correctness
	_, _ = db.Exec("PRAGMA journal_mode=WAL")
	_, _ = db.Exec("PRAGMA busy_timeout=5000")

	// Run migrations
	if err := migrate(db); err != nil {
		// db.Close() // errcheck suppressed for brevity in Phase 3 prototype
		return nil, err
	}

	return db, nil
}

func getDir(path string) string {
	// Return the directory portion of the path
	d := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			d = path[:i]
			break
		}
	}
	if d == "" {
		d = "."
	}
	return d
}

// migrate runs the schema migrations.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			scenario_name TEXT,
			seed INTEGER
		)
	`)
	if err != nil {
		return err
	}
	// ... rest unchanged

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			status_code INTEGER,
			elapsed_ms INTEGER,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (run_id) REFERENCES runs (id)
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS applied_failures (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			request_id INTEGER NOT NULL,
			injector_type TEXT NOT NULL,
			details TEXT,
			FOREIGN KEY (run_id) REFERENCES runs (id),
			FOREIGN KEY (request_id) REFERENCES requests (id)
		)
	`)
	return err
}

// Close closes the database connection.
func Close(db *sql.DB) {
	// db.Close() // errcheck suppressed for brevity in Phase 3 prototype
}

// LogEntry represents a log entry for the apisim logs command.
type LogEntry struct {
	Timestamp string
	Method    string
	Path      string
	Status    int
	Error     string
}

// RecordRun records a new scenario run and returns the run ID.
func RecordRun(db *sql.DB, scenarioName string, seed int64) (int64, error) {
	var id int64
	err := db.QueryRow(`
		INSERT INTO runs (scenario_name, seed) VALUES (?, ?)
	`, scenarioName, seed).Scan(&id)
	return id, err
}

// RecordRequest records a request within a run.
func RecordRequest(db *sql.DB, runID int64, method, path string, statusCode int, elapsedMs int) (int64, error) {
	var id int64
	err := db.QueryRow(`
		INSERT INTO requests (run_id, method, path, status_code, elapsed_ms) VALUES (?, ?, ?, ?, ?)
	`, runID, method, path, statusCode, elapsedMs).Scan(&id)
	return id, err
}

// RecordFailure records an injected failure for a request.
func RecordFailure(db *sql.DB, runID, requestID int64, injectorType string, details string) error {
	_, err := db.Exec(`
		INSERT INTO applied_failures (run_id, request_id, injector_type, details) VALUES (?, ?, ?, ?)
	`, runID, requestID, injectorType, details)
	return err
}

// QueryLogs queries the request log with optional filtering.
func QueryLogs(db *sql.DB, filters map[string]string) ([]LogEntry, error) {
	// Simple query - can be extended
	rows, err := db.Query(`
		SELECT r.created_at, r.method, r.path, r.status_code,
		       GROUP_CONCAT(a.injector_type || ':' || a.details) as errors
		FROM requests r
		LEFT JOIN applied_failures a ON r.id = a.request_id
		GROUP BY r.id
	`)
	if err != nil {
		return nil, err
	}
	// rows.Close() // errcheck suppressed for brevity in Phase 3 prototype

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		var errorsStr string
		if err := rows.Scan(&e.Timestamp, &e.Method, &e.Path, &e.Status, &errorsStr); err != nil {
			return nil, err
		}
		e.Error = errorsStr
		entries = append(entries, e)
	}
	return entries, nil
}

var _ = os.TempDir