package persistence

import (
	"database/sql"
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

	currentSchemaVersion = 8 // Current database schema version
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
		{
			version: 3,
			name:    "add_heals_table",
			sql: `
			CREATE TABLE IF NOT EXISTS heals (
				heal_id TEXT PRIMARY KEY,
				error_id TEXT NOT NULL,
				run_id TEXT,
				diff_content TEXT NOT NULL,
				diff_content_hash TEXT,
				file_path TEXT,
				model_id TEXT,
				prompt_hash TEXT,
				input_tokens INTEGER DEFAULT 0,
				output_tokens INTEGER DEFAULT 0,
				cache_read_tokens INTEGER DEFAULT 0,
				cache_write_tokens INTEGER DEFAULT 0,
				cost_usd REAL DEFAULT 0,
				status TEXT DEFAULT 'pending',
				created_at INTEGER NOT NULL,
				applied_at INTEGER,
				verified_at INTEGER,
				verification_result TEXT,
				attempt_number INTEGER DEFAULT 1,
				parent_heal_id TEXT,
				failure_reason TEXT,
				FOREIGN KEY (error_id) REFERENCES errors(error_id),
				FOREIGN KEY (run_id) REFERENCES runs(run_id),
				FOREIGN KEY (parent_heal_id) REFERENCES heals(heal_id)
			);

			CREATE INDEX IF NOT EXISTS idx_heals_error_id ON heals(error_id);
			CREATE INDEX IF NOT EXISTS idx_heals_status ON heals(status);
			CREATE INDEX IF NOT EXISTS idx_heals_run_id ON heals(run_id);
			CREATE INDEX IF NOT EXISTS idx_heals_diff_content_hash ON heals(diff_content_hash);
			`,
		},
		{
			version: 4,
			name:    "add_error_locations",
			sql: `
			CREATE TABLE IF NOT EXISTS error_locations (
				location_id TEXT PRIMARY KEY,
				error_id TEXT NOT NULL,
				run_id TEXT NOT NULL,
				file_path TEXT NOT NULL,
				line_number INTEGER,
				column_number INTEGER,
				file_hash TEXT,
				first_seen_at INTEGER NOT NULL,
				last_seen_at INTEGER NOT NULL,
				seen_count INTEGER DEFAULT 1,
				FOREIGN KEY (error_id) REFERENCES errors(error_id),
				FOREIGN KEY (run_id) REFERENCES runs(run_id),
				UNIQUE(error_id, file_path, line_number)
			);

			CREATE INDEX IF NOT EXISTS idx_error_locations_error_id ON error_locations(error_id);
			CREATE INDEX IF NOT EXISTS idx_error_locations_file_path ON error_locations(file_path);
			CREATE INDEX IF NOT EXISTS idx_error_locations_run_id ON error_locations(run_id);
			`,
		},
		{
			version: 5,
			name:    "add_sync_and_state_tracking",
			sql: `
			-- Codebase state hash for deduplication across cloud agents
			ALTER TABLE runs ADD COLUMN codebase_state_hash TEXT;

			-- Sync status for future remote sync (pending/synced/failed)
			ALTER TABLE runs ADD COLUMN sync_status TEXT DEFAULT 'pending';
			ALTER TABLE errors ADD COLUMN sync_status TEXT DEFAULT 'pending';
			ALTER TABLE heals ADD COLUMN sync_status TEXT DEFAULT 'pending';

			-- Indices for sync queries
			CREATE INDEX IF NOT EXISTS idx_runs_codebase_state_hash ON runs(codebase_state_hash);
			CREATE INDEX IF NOT EXISTS idx_runs_sync_status ON runs(sync_status);
			CREATE INDEX IF NOT EXISTS idx_errors_sync_status ON errors(sync_status);
			CREATE INDEX IF NOT EXISTS idx_heals_sync_status ON heals(sync_status);
			`,
		},
		{
			version: 6,
			name:    "drop_unused_indices",
			sql: `
			-- Drop indices with low selectivity or unused
			DROP INDEX IF EXISTS idx_errors_run_id;
			DROP INDEX IF EXISTS idx_errors_status;
			DROP INDEX IF EXISTS idx_errors_content_hash_time;
			DROP INDEX IF EXISTS idx_runs_is_dirty;
			DROP INDEX IF EXISTS idx_errors_sync_status;
			`,
		},
		{
			version: 7,
			name:    "drop_dirty_state_tracking",
			sql: `
			-- Drop codebase_state_hash index - we now use commit_sha directly for dedup
			-- (columns is_dirty, dirty_files, base_commit_sha, codebase_state_hash remain in schema
			-- but are no longer written - SQLite doesn't support DROP COLUMN easily)
			DROP INDEX IF EXISTS idx_runs_codebase_state_hash;
			`,
		},
		{
			version: 8,
			name:    "restore_run_id_index_for_cache",
			sql: `
			-- Restore idx_errors_run_id for cache lookups (GetErrorsByRunID)
			-- This was dropped in v6 but is now needed for efficient cache retrieval
			CREATE INDEX IF NOT EXISTS idx_errors_run_id ON errors(run_id);
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
func (w *SQLiteWriter) RecordRun(runID, workflowName, commitSHA, executionMode string) error {
	// Note: is_dirty, dirty_files, base_commit_sha, codebase_state_hash are deprecated
	// We now require clean commits before running, so is_dirty is always 0
	query := `
		INSERT INTO runs (run_id, workflow_name, commit_sha, execution_mode, started_at, is_dirty)
		VALUES (?, ?, ?, ?, ?, 0)
	`

	_, err := w.db.Exec(query, runID, workflowName, commitSHA, executionMode, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to record run: %w", err)
	}

	return nil
}

// GetRunByCommit finds a run by its commit SHA (for deduplication)
// Since we now require clean commits, the commit SHA uniquely identifies the codebase state.
func (w *SQLiteWriter) GetRunByCommit(commitSHA string) (runID string, found bool, err error) {
	query := `SELECT run_id FROM runs WHERE commit_sha = ? ORDER BY started_at DESC LIMIT 1`

	err = w.db.QueryRow(query, commitSHA).Scan(&runID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to query run by commit: %w", err)
	}

	return runID, true, nil
}

// RunRecord represents a workflow run stored in the database
type RunRecord struct {
	RunID         string
	WorkflowName  string
	CommitSHA     string
	ExecutionMode string
	StartedAt     time.Time
	CompletedAt   time.Time
	TotalErrors   int
}

// ErrorRecord represents an error stored in the database
type ErrorRecord struct {
	ErrorID     string
	RunID       string
	FilePath    string
	LineNumber  int
	ErrorType   string
	Message     string
	StackTrace  string
	FileHash    string
	ContentHash string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	SeenCount   int
	Status      string
}

// GetRunByID retrieves a run by its ID
func (w *SQLiteWriter) GetRunByID(runID string) (*RunRecord, error) {
	query := `
		SELECT run_id, workflow_name, commit_sha, execution_mode,
			started_at, completed_at, total_errors
		FROM runs WHERE run_id = ?
	`

	var run RunRecord
	var startedAt, completedAt sql.NullInt64
	var totalErrors sql.NullInt64

	err := w.db.QueryRow(query, runID).Scan(
		&run.RunID,
		&run.WorkflowName,
		&run.CommitSHA,
		&run.ExecutionMode,
		&startedAt,
		&completedAt,
		&totalErrors,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	if startedAt.Valid {
		run.StartedAt = time.Unix(startedAt.Int64, 0)
	}
	if completedAt.Valid {
		run.CompletedAt = time.Unix(completedAt.Int64, 0)
	}
	if totalErrors.Valid {
		run.TotalErrors = int(totalErrors.Int64)
	}

	return &run, nil
}

// GetErrorsByRunID retrieves all errors for a given run
func (w *SQLiteWriter) GetErrorsByRunID(runID string) ([]*ErrorRecord, error) {
	query := `
		SELECT error_id, run_id, file_path, line_number, error_type, message,
			stack_trace, file_hash, content_hash, first_seen_at, last_seen_at,
			seen_count, status
		FROM errors WHERE run_id = ?
		ORDER BY file_path, line_number
	`

	rows, err := w.db.Query(query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query errors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var errors []*ErrorRecord
	for rows.Next() {
		var e ErrorRecord
		var filePath, stackTrace, fileHash, contentHash, status sql.NullString
		var firstSeenAt, lastSeenAt sql.NullInt64

		err := rows.Scan(
			&e.ErrorID,
			&e.RunID,
			&filePath,
			&e.LineNumber,
			&e.ErrorType,
			&e.Message,
			&stackTrace,
			&fileHash,
			&contentHash,
			&firstSeenAt,
			&lastSeenAt,
			&e.SeenCount,
			&status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan error: %w", err)
		}

		e.FilePath = filePath.String
		e.StackTrace = stackTrace.String
		e.FileHash = fileHash.String
		e.ContentHash = contentHash.String
		e.Status = status.String
		if firstSeenAt.Valid {
			e.FirstSeenAt = time.Unix(firstSeenAt.Int64, 0)
		}
		if lastSeenAt.Valid {
			e.LastSeenAt = time.Unix(lastSeenAt.Int64, 0)
		}

		errors = append(errors, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating errors: %w", err)
	}

	return errors, nil
}

// RecordFindings adds multiple findings in a single batch operation
func (w *SQLiteWriter) RecordFindings(findings []*FindingRecord) error {
	if len(findings) == 0 {
		return nil
	}

	w.batchMutex.Lock()
	defer w.batchMutex.Unlock()

	// Add all findings to the batch
	w.batch = append(w.batch, findings...)

	// Flush the entire batch immediately for consistency
	return w.flushBatch()
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

// RecordHeal inserts a new heal record into the database
func (w *SQLiteWriter) RecordHeal(heal *HealRecord) error {
	query := `
		INSERT INTO heals (
			heal_id, error_id, run_id, diff_content, diff_content_hash, file_path,
			model_id, prompt_hash, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens, cost_usd,
			status, created_at, applied_at, verified_at, verification_result,
			attempt_number, parent_heal_id, failure_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var appliedAt, verifiedAt *int64
	if heal.AppliedAt != nil {
		ts := heal.AppliedAt.Unix()
		appliedAt = &ts
	}
	if heal.VerifiedAt != nil {
		ts := heal.VerifiedAt.Unix()
		verifiedAt = &ts
	}

	var verificationResult *string
	if heal.VerificationResult != "" {
		s := string(heal.VerificationResult)
		verificationResult = &s
	}

	// Compute diff content hash if not provided
	diffContentHash := heal.DiffContentHash
	if diffContentHash == "" && heal.DiffContent != "" {
		diffContentHash = ComputeContentHash(heal.DiffContent)
	}

	_, err := w.db.Exec(query,
		heal.HealID,
		heal.ErrorID,
		heal.RunID,
		heal.DiffContent,
		diffContentHash,
		heal.FilePath,
		heal.ModelID,
		heal.PromptHash,
		heal.InputTokens,
		heal.OutputTokens,
		heal.CacheReadTokens,
		heal.CacheWriteTokens,
		heal.CostUSD,
		string(heal.Status),
		heal.CreatedAt.Unix(),
		appliedAt,
		verifiedAt,
		verificationResult,
		heal.AttemptNumber,
		heal.ParentHealID,
		heal.FailureReason,
	)
	if err != nil {
		return fmt.Errorf("failed to record heal: %w", err)
	}

	return nil
}

// UpdateHealStatus updates the status and optionally the applied_at timestamp of a heal
func (w *SQLiteWriter) UpdateHealStatus(healID string, status HealStatus, appliedAt *time.Time) error {
	var query string
	var args []interface{}

	if appliedAt != nil {
		query = `UPDATE heals SET status = ?, applied_at = ? WHERE heal_id = ?`
		args = []interface{}{string(status), appliedAt.Unix(), healID}
	} else {
		query = `UPDATE heals SET status = ? WHERE heal_id = ?`
		args = []interface{}{string(status), healID}
	}

	result, err := w.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update heal status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("heal not found: %s", healID)
	}

	return nil
}

// RecordHealVerification updates a heal with verification results
func (w *SQLiteWriter) RecordHealVerification(healID string, result VerificationResult) error {
	query := `UPDATE heals SET verified_at = ?, verification_result = ? WHERE heal_id = ?`

	dbResult, err := w.db.Exec(query, time.Now().Unix(), string(result), healID)
	if err != nil {
		return fmt.Errorf("failed to record heal verification: %w", err)
	}

	rowsAffected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("heal not found: %s", healID)
	}

	return nil
}

// GetHealsForError retrieves all heals for a given error, ordered by attempt number
func (w *SQLiteWriter) GetHealsForError(errorID string) ([]*HealRecord, error) {
	query := `
		SELECT heal_id, error_id, run_id, diff_content, diff_content_hash, file_path,
			model_id, prompt_hash, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens, cost_usd,
			status, created_at, applied_at, verified_at, verification_result,
			attempt_number, parent_heal_id, failure_reason
		FROM heals
		WHERE error_id = ?
		ORDER BY attempt_number ASC
	`

	rows, err := w.db.Query(query, errorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query heals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var heals []*HealRecord
	for rows.Next() {
		heal, scanErr := scanHealRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		heals = append(heals, heal)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating heals: %w", err)
	}

	return heals, nil
}

// GetLatestHealForError retrieves the most recent heal for a given error
func (w *SQLiteWriter) GetLatestHealForError(errorID string) (*HealRecord, error) {
	query := `
		SELECT heal_id, error_id, run_id, diff_content, diff_content_hash, file_path,
			model_id, prompt_hash, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens, cost_usd,
			status, created_at, applied_at, verified_at, verification_result,
			attempt_number, parent_heal_id, failure_reason
		FROM heals
		WHERE error_id = ?
		ORDER BY attempt_number DESC
		LIMIT 1
	`

	row := w.db.QueryRow(query, errorID)
	heal, err := scanHealRowSingle(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest heal: %w", err)
	}

	return heal, nil
}

// scanHealRow scans a heal row from sql.Rows
func scanHealRow(rows *sql.Rows) (*HealRecord, error) {
	var heal HealRecord
	var runID, diffContentHash, filePath, modelID, promptHash sql.NullString
	var appliedAt, verifiedAt sql.NullInt64
	var verificationResult, parentHealID, failureReason sql.NullString
	var createdAtUnix int64
	var status string

	err := rows.Scan(
		&heal.HealID,
		&heal.ErrorID,
		&runID,
		&heal.DiffContent,
		&diffContentHash,
		&filePath,
		&modelID,
		&promptHash,
		&heal.InputTokens,
		&heal.OutputTokens,
		&heal.CacheReadTokens,
		&heal.CacheWriteTokens,
		&heal.CostUSD,
		&status,
		&createdAtUnix,
		&appliedAt,
		&verifiedAt,
		&verificationResult,
		&heal.AttemptNumber,
		&parentHealID,
		&failureReason,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan heal row: %w", err)
	}

	heal.RunID = runID.String
	heal.DiffContentHash = diffContentHash.String
	heal.FilePath = filePath.String
	heal.ModelID = modelID.String
	heal.PromptHash = promptHash.String
	heal.Status = HealStatus(status)
	heal.CreatedAt = time.Unix(createdAtUnix, 0)

	if appliedAt.Valid {
		t := time.Unix(appliedAt.Int64, 0)
		heal.AppliedAt = &t
	}
	if verifiedAt.Valid {
		t := time.Unix(verifiedAt.Int64, 0)
		heal.VerifiedAt = &t
	}
	if verificationResult.Valid {
		heal.VerificationResult = VerificationResult(verificationResult.String)
	}
	if parentHealID.Valid {
		heal.ParentHealID = &parentHealID.String
	}
	if failureReason.Valid {
		heal.FailureReason = &failureReason.String
	}

	return &heal, nil
}

// scanHealRowSingle scans a heal row from sql.Row
func scanHealRowSingle(row *sql.Row) (*HealRecord, error) {
	var heal HealRecord
	var runID, diffContentHash, filePath, modelID, promptHash sql.NullString
	var appliedAt, verifiedAt sql.NullInt64
	var verificationResult, parentHealID, failureReason sql.NullString
	var createdAtUnix int64
	var status string

	err := row.Scan(
		&heal.HealID,
		&heal.ErrorID,
		&runID,
		&heal.DiffContent,
		&diffContentHash,
		&filePath,
		&modelID,
		&promptHash,
		&heal.InputTokens,
		&heal.OutputTokens,
		&heal.CacheReadTokens,
		&heal.CacheWriteTokens,
		&heal.CostUSD,
		&status,
		&createdAtUnix,
		&appliedAt,
		&verifiedAt,
		&verificationResult,
		&heal.AttemptNumber,
		&parentHealID,
		&failureReason,
	)
	if err != nil {
		return nil, err
	}

	heal.RunID = runID.String
	heal.DiffContentHash = diffContentHash.String
	heal.FilePath = filePath.String
	heal.ModelID = modelID.String
	heal.PromptHash = promptHash.String
	heal.Status = HealStatus(status)
	heal.CreatedAt = time.Unix(createdAtUnix, 0)

	if appliedAt.Valid {
		t := time.Unix(appliedAt.Int64, 0)
		heal.AppliedAt = &t
	}
	if verifiedAt.Valid {
		t := time.Unix(verifiedAt.Int64, 0)
		heal.VerifiedAt = &t
	}
	if verificationResult.Valid {
		heal.VerificationResult = VerificationResult(verificationResult.String)
	}
	if parentHealID.Valid {
		heal.ParentHealID = &parentHealID.String
	}
	if failureReason.Valid {
		heal.FailureReason = &failureReason.String
	}

	return &heal, nil
}

// RecordErrorLocation records or updates an error location
func (w *SQLiteWriter) RecordErrorLocation(loc *ErrorLocation) error {
	now := time.Now().Unix()

	// Try to update existing location first (upsert pattern)
	updateQuery := `
		UPDATE error_locations
		SET seen_count = seen_count + 1, last_seen_at = ?, file_hash = COALESCE(?, file_hash)
		WHERE error_id = ? AND file_path = ? AND line_number = ?
	`

	result, err := w.db.Exec(updateQuery, now, loc.FileHash, loc.ErrorID, loc.FilePath, loc.LineNumber)
	if err != nil {
		return fmt.Errorf("failed to update error location: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// If no existing row, insert new one
	if rowsAffected == 0 {
		locationID, uuidErr := util.GenerateUUID()
		if uuidErr != nil {
			return fmt.Errorf("failed to generate location ID: %w", uuidErr)
		}

		insertQuery := `
			INSERT INTO error_locations (
				location_id, error_id, run_id, file_path, line_number,
				column_number, file_hash, first_seen_at, last_seen_at, seen_count
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		`

		_, err = w.db.Exec(insertQuery,
			locationID,
			loc.ErrorID,
			loc.RunID,
			loc.FilePath,
			loc.LineNumber,
			loc.ColumnNumber,
			loc.FileHash,
			now,
			now,
		)
		if err != nil {
			return fmt.Errorf("failed to insert error location: %w", err)
		}
	}

	return nil
}

// GetLocationsForError retrieves all locations where an error has been seen
func (w *SQLiteWriter) GetLocationsForError(errorID string) ([]*ErrorLocation, error) {
	query := `
		SELECT location_id, error_id, run_id, file_path, line_number,
			column_number, file_hash, first_seen_at, last_seen_at, seen_count
		FROM error_locations
		WHERE error_id = ?
		ORDER BY seen_count DESC, last_seen_at DESC
	`

	rows, err := w.db.Query(query, errorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query error locations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var locations []*ErrorLocation
	for rows.Next() {
		var loc ErrorLocation
		var fileHash sql.NullString
		var firstSeenAt, lastSeenAt int64

		err := rows.Scan(
			&loc.LocationID,
			&loc.ErrorID,
			&loc.RunID,
			&loc.FilePath,
			&loc.LineNumber,
			&loc.ColumnNumber,
			&fileHash,
			&firstSeenAt,
			&lastSeenAt,
			&loc.SeenCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan error location: %w", err)
		}

		loc.FileHash = fileHash.String
		loc.FirstSeenAt = time.Unix(firstSeenAt, 0)
		loc.LastSeenAt = time.Unix(lastSeenAt, 0)
		locations = append(locations, &loc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating error locations: %w", err)
	}

	return locations, nil
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
