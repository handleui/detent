package persistence

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

const (
	spendDBFile           = "spend.db"
	spendSchemaVersion    = 1
)

// SpendDB handles global spend tracking across all repositories.
// Unlike SQLiteWriter which is per-repo, this tracks spend globally
// in ~/.detent/spend.db for accurate monthly budget enforcement.
type SpendDB struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// spendDB is the singleton global spend database
var (
	globalSpendDB   *SpendDB
	globalSpendOnce sync.Once
	errGlobalSpend  error
)

// GetSpendDB returns the global spend database singleton.
// The database is created lazily and kept open for the lifetime of the process.
func GetSpendDB() (*SpendDB, error) {
	globalSpendOnce.Do(func() {
		globalSpendDB, errGlobalSpend = openSpendDB()
	})
	return globalSpendDB, errGlobalSpend
}

// openSpendDB opens (or creates) the global spend database.
func openSpendDB() (*SpendDB, error) {
	detentDir, err := GetDetentDir()
	if err != nil {
		return nil, err
	}

	// Ensure detent directory exists
	// #nosec G301 - 0700 is intentionally restrictive
	if mkdirErr := os.MkdirAll(detentDir, 0o700); mkdirErr != nil {
		return nil, fmt.Errorf("creating detent directory: %w", mkdirErr)
	}

	dbPath := filepath.Join(detentDir, spendDBFile)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening spend database: %w", err)
	}

	// Configure connection pool - single connection for SQLite
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Apply performance pragmas
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("executing %s: %w", pragma, err)
		}
	}

	sdb := &SpendDB{db: db, path: dbPath}

	// Initialize schema
	if err := sdb.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing spend schema: %w", err)
	}

	// Set secure permissions
	// #nosec G302 - intentionally restrictive permissions
	if err := os.Chmod(dbPath, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting spend database permissions: %w", err)
	}

	return sdb, nil
}

// initSchema creates the spend_log table if needed.
func (s *SpendDB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS spend_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		cost_usd REAL NOT NULL,
		repo_id TEXT,
		created_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_spend_log_created_at ON spend_log(created_at);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("creating spend schema: %w", err)
	}

	// Check/update schema version
	var version int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version); err != nil {
		return fmt.Errorf("querying schema version: %w", err)
	}

	if version < spendSchemaVersion {
		if _, err := s.db.Exec("INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
			spendSchemaVersion, time.Now().Unix()); err != nil {
			return fmt.Errorf("recording schema version: %w", err)
		}
	}

	return nil
}

// RecordSpend records a spend amount to the global spend log.
// repoID is optional and used for auditing purposes.
func (s *SpendDB) RecordSpend(costUSD float64, repoID string) error {
	if costUSD <= 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO spend_log (cost_usd, repo_id, created_at) VALUES (?, ?, ?)`
	_, err := s.db.Exec(query, costUSD, repoID, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("recording spend: %w", err)
	}

	return nil
}

// GetMonthlySpend returns the total spend for the given month (format: "2006-01").
// If month is empty, uses the current month.
func (s *SpendDB) GetMonthlySpend(month string) (float64, error) {
	if month == "" {
		month = time.Now().Format("2006-01")
	}

	start, err := time.Parse("2006-01", month)
	if err != nil {
		return 0, fmt.Errorf("invalid month format: %w", err)
	}
	end := start.AddDate(0, 1, 0)

	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT COALESCE(SUM(cost_usd), 0) FROM spend_log WHERE created_at >= ? AND created_at < ?`
	var total float64
	if err := s.db.QueryRow(query, start.Unix(), end.Unix()).Scan(&total); err != nil {
		return 0, fmt.Errorf("querying monthly spend: %w", err)
	}

	return total, nil
}

// Close closes the spend database connection.
func (s *SpendDB) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
