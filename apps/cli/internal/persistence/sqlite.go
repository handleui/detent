package persistence

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/detent/cli/internal/util"
)

const (
	detentDir    = ".detent"
	detentDBName = "detent.db"
	batchSize    = 500 // Batch size for error inserts

	currentSchemaVersion = 2 // Current database schema version
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

	// Apply performance pragmas for 2-5x speedup
	pragmas := []string{
		"PRAGMA journal_mode=WAL",    // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous=NORMAL",  // Faster writes, still safe with WAL
		"PRAGMA cache_size=-64000",   // 64MB cache for better performance
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			if closeErr := db.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to execute %s: %w (additionally, failed to close database: %v)", pragma, err, closeErr)
			}
			return nil, fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
	}

	writer := &SQLiteWriter{
		db:    db,
		path:  dbPath,
		batch: make([]*FindingRecord, 0, batchSize),
	}

	// Initialize schema (this creates the database file if it doesn't exist)
	if err := writer.initSchema(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to initialize schema: %w (additionally, failed to close database: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Set secure file permissions on database file (owner read/write only)
	// This must be done after schema initialization which creates the file
	// #nosec G302 - intentionally setting restrictive permissions
	if err := os.Chmod(dbPath, 0o600); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to set database permissions: %w (additionally, failed to close database: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to set database permissions: %w", err)
	}

	return writer, nil
}

// initSchema creates the database tables and indices
func (w *SQLiteWriter) initSchema() error {
	// Create schema_version table first
	versionTableSchema := `
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	);
	`

	if _, err := w.db.Exec(versionTableSchema); err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	// Get current schema version
	var version int
	err := w.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to query schema version: %w", err)
	}

	// Apply migrations if needed
	if version < currentSchemaVersion {
		if err := w.applyMigrations(version); err != nil {
			return fmt.Errorf("failed to apply migrations: %w", err)
		}
	}

	return nil
}

// applyMigrations applies database migrations from the current version to the latest
func (w *SQLiteWriter) applyMigrations(fromVersion int) error {
	migrations := []struct {
		version int
		name    string
		sql     string
	}{
		{
			version: 1,
			name:    "initial_schema",
			sql: `
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
			CREATE INDEX IF NOT EXISTS idx_errors_content_hash_run_id ON errors(content_hash, run_id);
			CREATE INDEX IF NOT EXISTS idx_errors_content_hash_time ON errors(content_hash, first_seen_at DESC);
			CREATE INDEX IF NOT EXISTS idx_errors_file_path ON errors(file_path);
			CREATE INDEX IF NOT EXISTS idx_errors_status ON errors(status);
			`,
		},
		{
			version: 2,
			name:    "add_worktree_tracking",
			sql: `
			ALTER TABLE runs ADD COLUMN is_dirty INTEGER DEFAULT 0;
			ALTER TABLE runs ADD COLUMN dirty_files TEXT;
			ALTER TABLE runs ADD COLUMN base_commit_sha TEXT;

			CREATE INDEX IF NOT EXISTS idx_runs_is_dirty ON runs(is_dirty);
			`,
		},
	}

	// Apply each migration in a transaction
	for _, migration := range migrations {
		if migration.version <= fromVersion {
			continue
		}

		tx, err := w.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration v%d: %w", migration.version, err)
		}

		// Execute migration SQL
		if _, err := tx.Exec(migration.sql); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to execute migration v%d (%s): %w (additionally, failed to rollback: %v)",
					migration.version, migration.name, err, rbErr)
			}
			return fmt.Errorf("failed to execute migration v%d (%s): %w", migration.version, migration.name, err)
		}

		// Record migration in schema_version table
		if _, err := tx.Exec("INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
			migration.version, time.Now().Unix()); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to record migration v%d: %w (additionally, failed to rollback: %v)",
					migration.version, err, rbErr)
			}
			return fmt.Errorf("failed to record migration v%d: %w", migration.version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration v%d: %w", migration.version, err)
		}
	}

	return nil
}

// RecordRun inserts a new run record into the database
func (w *SQLiteWriter) RecordRun(runID, workflowName, commitSHA, executionMode string, isDirty bool, dirtyFiles []string) error {
	// Marshal dirty files to JSON
	var dirtyFilesJSON *string
	if len(dirtyFiles) > 0 {
		jsonBytes, err := json.Marshal(dirtyFiles)
		if err != nil {
			return fmt.Errorf("failed to marshal dirty files: %w", err)
		}
		jsonStr := string(jsonBytes)
		dirtyFilesJSON = &jsonStr
	}

	// Convert isDirty bool to integer (0 or 1)
	isDirtyInt := 0
	if isDirty {
		isDirtyInt = 1
	}

	query := `
		INSERT INTO runs (run_id, workflow_name, commit_sha, execution_mode, started_at, is_dirty, dirty_files, base_commit_sha)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := w.db.Exec(query, runID, workflowName, commitSHA, executionMode, time.Now().Unix(), isDirtyInt, dirtyFilesJSON, commitSHA)
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
	var insertStmt, updateStmt *sql.Stmt
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

	now := time.Now().Unix()

	// Compute content hashes for all findings and build lookup map
	contentHashes := make([]string, 0, len(w.batch))
	hashToFindings := make(map[string][]*FindingRecord)

	for _, finding := range w.batch {
		contentHash := ComputeContentHash(finding.Message)
		if _, exists := hashToFindings[contentHash]; !exists {
			contentHashes = append(contentHashes, contentHash)
		}
		hashToFindings[contentHash] = append(hashToFindings[contentHash], finding)
	}

	// Batch lookup existing errors using WHERE IN clause
	existingErrors := make(map[string]string) // content_hash -> error_id

	if len(contentHashes) > 0 {
		// Build query with placeholders for IN clause
		placeholders := make([]string, len(contentHashes))
		args := make([]interface{}, len(contentHashes))
		for i, hash := range contentHashes {
			placeholders[i] = "?"
			args[i] = hash
		}

		// #nosec G201 - SQL string formatting with placeholders only (not user data)
		query := fmt.Sprintf(`
			SELECT content_hash, error_id
			FROM errors
			WHERE content_hash IN (%s)
		`, joinStrings(placeholders, ","))

		rows, queryErr := tx.Query(query, args...)
		if queryErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to query existing errors: %w (additionally, failed to rollback: %v)", queryErr, rbErr)
			}
			return fmt.Errorf("failed to query existing errors: %w", queryErr)
		}

		for rows.Next() {
			var contentHash, errorID string
			if scanErr := rows.Scan(&contentHash, &errorID); scanErr != nil {
				if closeErr := rows.Close(); closeErr != nil {
					return fmt.Errorf("failed to scan existing error: %w (additionally, failed to close rows: %v)", scanErr, closeErr)
				}
				if rbErr := tx.Rollback(); rbErr != nil {
					return fmt.Errorf("failed to scan existing error: %w (additionally, failed to rollback: %v)", scanErr, rbErr)
				}
				return fmt.Errorf("failed to scan existing error: %w", scanErr)
			}
			existingErrors[contentHash] = errorID
		}
		if closeErr := rows.Close(); closeErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to close rows: %w (additionally, failed to rollback: %v)", closeErr, rbErr)
			}
			return fmt.Errorf("failed to close rows: %w", closeErr)
		}
	}

	// Process all findings using the batched lookup results
	for _, finding := range w.batch {
		contentHash := ComputeContentHash(finding.Message)

		if existingID, exists := existingErrors[contentHash]; exists {
			// Update existing error
			_, updateErr := updateStmt.Exec(now, existingID)
			if updateErr != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					return fmt.Errorf("failed to update error: %w (additionally, failed to rollback: %v)", updateErr, rbErr)
				}
				return fmt.Errorf("failed to update error: %w", updateErr)
			}
		} else {
			// Insert new error
			errorID, uuidErr := util.GenerateUUID()
			if uuidErr != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					return fmt.Errorf("failed to generate error ID: %w (additionally, failed to rollback: %v)", uuidErr, rbErr)
				}
				return fmt.Errorf("failed to generate error ID: %w", uuidErr)
			}
			_, execErr := insertStmt.Exec(
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
			if execErr != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					return fmt.Errorf("failed to insert error: %w (additionally, failed to rollback: %v)", execErr, rbErr)
				}
				return fmt.Errorf("failed to insert error: %w", execErr)
			}
			w.errorCount++
			// Add to map so subsequent duplicates in same batch are handled correctly
			existingErrors[contentHash] = errorID
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

// joinStrings is a simple string join helper
func joinStrings(strs []string, sep string) string {
	return strings.Join(strs, sep)
}
