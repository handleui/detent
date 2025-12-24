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
	detentDir    = ".detent"
	detentDBName = "detent.db"
	batchSize    = 500 // Batch size for error inserts
)

// createDirIfNotExists creates a directory if it doesn't exist
func createDirIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// #nosec G301 - standard permissions for app data directory
		return os.MkdirAll(path, 0o755)
	}
	return nil
}

// SQLiteWriter handles writing scan results to SQLite database
type SQLiteWriter struct {
	db         *sql.DB
	path       string
	batch      []*FindingRecord
	batchMutex sync.Mutex
	errorCount int
}

// NewSQLiteWriter creates a new SQLite writer and initializes the database schema
func NewSQLiteWriter(repoRoot string) (*SQLiteWriter, error) {
	// Create .detent directory
	detentPath := filepath.Join(repoRoot, detentDir)

	// #nosec G301 - standard permissions for app data directory
	if err := createDirIfNotExists(detentPath); err != nil {
		return nil, fmt.Errorf("failed to create .detent directory: %w", err)
	}

	// Open SQLite database
	dbPath := filepath.Join(detentPath, detentDBName)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	writer := &SQLiteWriter{
		db:    db,
		path:  dbPath,
		batch: make([]*FindingRecord, 0, batchSize),
	}

	// Initialize schema
	if err := writer.initSchema(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to initialize schema: %w (additionally, failed to close database: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return writer, nil
}

// initSchema creates the database tables and indices
func (w *SQLiteWriter) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS runs (
		run_id TEXT PRIMARY KEY,
		workflow_name TEXT,
		commit_sha TEXT,
		execution_mode TEXT,
		started_at INTEGER,
		completed_at INTEGER,
		total_errors INTEGER
	);

	CREATE TABLE IF NOT EXISTS errors (
		error_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		file_path TEXT,
		line_number INTEGER,
		error_type TEXT,
		message TEXT NOT NULL,
		stack_trace TEXT,
		file_hash TEXT,
		content_hash TEXT,
		first_seen_at INTEGER,
		last_seen_at INTEGER,
		seen_count INTEGER DEFAULT 1,
		status TEXT DEFAULT 'open',
		FOREIGN KEY (run_id) REFERENCES runs(run_id)
	);

	CREATE INDEX IF NOT EXISTS idx_errors_run_id ON errors(run_id);
	CREATE INDEX IF NOT EXISTS idx_errors_content_hash ON errors(content_hash);
	CREATE INDEX IF NOT EXISTS idx_errors_content_hash_time ON errors(content_hash, first_seen_at DESC);
	CREATE INDEX IF NOT EXISTS idx_errors_file_path ON errors(file_path);
	CREATE INDEX IF NOT EXISTS idx_errors_status ON errors(status);
	`

	_, err := w.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to execute schema creation statements: %w", err)
	}
	return nil
}

// RecordRun inserts a new run record into the database
func (w *SQLiteWriter) RecordRun(runID, workflowName, commitSHA, executionMode string) error {
	query := `
		INSERT INTO runs (run_id, workflow_name, commit_sha, execution_mode, started_at)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := w.db.Exec(query, runID, workflowName, commitSHA, executionMode, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to record run: %w", err)
	}

	return nil
}

// RecordError adds an error to the batch and flushes when batch size is reached
func (w *SQLiteWriter) RecordError(finding *FindingRecord) error {
	w.batchMutex.Lock()
	defer w.batchMutex.Unlock()

	w.batch = append(w.batch, finding)

	if len(w.batch) >= batchSize {
		return w.flushBatch()
	}

	return nil
}

// flushBatch processes all batched errors in a single transaction
// Must be called with batchMutex locked
func (w *SQLiteWriter) flushBatch() (err error) {
	if len(w.batch) == 0 {
		return nil
	}

	tx, err := w.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Setup cleanup function to handle rollback and statement closing
	var insertStmt, updateStmt, selectStmt *sql.Stmt
	defer func() {
		// Close prepared statements (best effort)
		if insertStmt != nil {
			if closeErr := insertStmt.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("failed to close insert statement: %w", closeErr)
			}
		}
		if updateStmt != nil {
			if closeErr := updateStmt.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("failed to close update statement: %w", closeErr)
			}
		}
		if selectStmt != nil {
			if closeErr := selectStmt.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("failed to close select statement: %w", closeErr)
			}
		}
	}()

	insertStmt, err = tx.Prepare(`
		INSERT INTO errors (
			error_id, run_id, file_path, line_number, error_type, message,
			stack_trace, file_hash, content_hash, first_seen_at, last_seen_at,
			seen_count, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'open')
	`)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("failed to prepare insert statement: %w (additionally, failed to rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}

	updateStmt, err = tx.Prepare(`
		UPDATE errors
		SET seen_count = seen_count + 1, last_seen_at = ?
		WHERE error_id = ?
	`)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("failed to prepare update statement: %w (additionally, failed to rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}

	selectStmt, err = tx.Prepare(`
		SELECT error_id FROM errors WHERE content_hash = ? ORDER BY first_seen_at DESC LIMIT 1
	`)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("failed to prepare select statement: %w (additionally, failed to rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("failed to prepare select statement: %w", err)
	}

	now := time.Now().Unix()

	for _, finding := range w.batch {
		contentHash := ComputeContentHash(finding.Message)

		var existingID string
		err := selectStmt.QueryRow(contentHash).Scan(&existingID)

		switch {
		case err == sql.ErrNoRows:
			errorID := generateUUID()
			_, err = insertStmt.Exec(
				errorID,
				finding.RunID,
				finding.FilePath,
				finding.Line,
				finding.Category,
				finding.Message,
				finding.StackTrace,
				finding.FileHash,
				contentHash,
				now,
				now,
			)
			if err != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					return fmt.Errorf("failed to insert error: %w (additionally, failed to rollback: %v)", err, rbErr)
				}
				return fmt.Errorf("failed to insert error: %w", err)
			}
			w.errorCount++
		case err != nil:
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to check for existing error: %w (additionally, failed to rollback: %v)", err, rbErr)
			}
			return fmt.Errorf("failed to check for existing error: %w", err)
		default:
			_, err = updateStmt.Exec(now, existingID)
			if err != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					return fmt.Errorf("failed to update error: %w (additionally, failed to rollback: %v)", err, rbErr)
				}
				return fmt.Errorf("failed to update error: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	w.batch = w.batch[:0]
	return nil
}

// FlushBatch flushes any remaining batched errors (public method for external callers)
func (w *SQLiteWriter) FlushBatch() error {
	w.batchMutex.Lock()
	defer w.batchMutex.Unlock()
	return w.flushBatch()
}

// FinalizeRun updates the run record with completion information
func (w *SQLiteWriter) FinalizeRun(runID string, totalErrors int) error {
	// Flush any remaining batched errors
	if err := w.FlushBatch(); err != nil {
		return fmt.Errorf("failed to flush batch: %w", err)
	}

	// Update run record
	updateQuery := `
		UPDATE runs
		SET completed_at = ?, total_errors = ?
		WHERE run_id = ?
	`

	_, err := w.db.Exec(updateQuery, time.Now().Unix(), totalErrors, runID)
	if err != nil {
		return fmt.Errorf("failed to finalize run: %w", err)
	}

	return nil
}

// GetErrorCount returns the current error count
func (w *SQLiteWriter) GetErrorCount() int {
	w.batchMutex.Lock()
	defer w.batchMutex.Unlock()
	return w.errorCount
}

// Close closes the database connection
func (w *SQLiteWriter) Close() error {
	if w.db != nil {
		return w.db.Close()
	}
	return nil
}

// Path returns the absolute path to the SQLite database file
func (w *SQLiteWriter) Path() string {
	return w.path
}
