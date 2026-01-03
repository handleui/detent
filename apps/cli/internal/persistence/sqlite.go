package persistence

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/detent/cli/internal/util"
	"github.com/detentsh/core/git"
)

const (
	batchSize = 500 // Batch size for error inserts

	currentSchemaVersion = 18 // Current database schema version

	// healSelectColumns is the standard column list for heal queries (must match scanHealFromScanner order)
	healSelectColumns = `heal_id, error_id, run_id, diff_content, diff_content_hash, file_path, file_hash,
		model_id, prompt_hash, input_tokens, output_tokens,
		cache_read_tokens, cache_write_tokens, cost_usd,
		status, created_at, applied_at, verified_at, verification_result,
		attempt_number, parent_heal_id, failure_reason`
)

// repoIDCache caches computed repository IDs to avoid repeated git subprocess calls.
// Key: absolute repo root path, Value: computed repo ID string.
var repoIDCache sync.Map

// ErrHealLockHeld is returned when a heal lock cannot be acquired because
// another process is already holding it.
var ErrHealLockHeld = errors.New("heal lock is held by another process")

func createDirIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// #nosec G301 - restrictive permissions for cache directory (owner-only access)
		return os.MkdirAll(path, 0o700)
	}
	return nil
}

// ComputeRepoID computes a stable identifier for a repository.
// Priority: 1) git remote URL, 2) first commit SHA, 3) repo path
// Returns a 16-character hex string suitable for directory names.
// Results are cached to avoid repeated git subprocess calls.
func ComputeRepoID(repoRoot string) (string, error) {
	// Normalize path for consistent cache key
	absPath, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repo path: %w", err)
	}

	// Check cache first
	if cached, ok := repoIDCache.Load(absPath); ok {
		if repoID, valid := cached.(string); valid {
			return repoID, nil
		}
	}

	// Compute the repo ID
	repoID := computeRepoIDUncached(absPath)

	// Store in cache
	repoIDCache.Store(absPath, repoID)
	return repoID, nil
}

// computeRepoIDUncached performs the actual repo ID computation without caching.
func computeRepoIDUncached(absPath string) string {
	// Priority 1: git remote origin URL (works across machines)
	remoteURL, err := git.GetRemoteURL(absPath)
	if err == nil && remoteURL != "" {
		// Normalize: strip .git suffix for consistent IDs
		remoteURL = strings.TrimSuffix(remoteURL, ".git")
		return hashToID(remoteURL)
	}

	// Priority 2: first commit SHA (immutable, works offline)
	firstCommit, err := git.GetFirstCommitSHA(absPath)
	if err == nil && firstCommit != "" {
		return hashToID(firstCommit)
	}

	// Priority 3: repo path (last resort - breaks if repo moves)
	return hashToID(absPath)
}

// hashToID computes a SHA256 hash and returns the first 20 hex characters (80 bits).
func hashToID(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])[:20]
}

// GetDatabasePath returns the path to the SQLite database for a given repo.
// Uses the consolidated directory: ~/.detent/repos/<repoID>.db
func GetDatabasePath(repoRoot string) (string, error) {
	detentDir, err := GetDetentDir()
	if err != nil {
		return "", err
	}

	repoID, err := ComputeRepoID(repoRoot)
	if err != nil {
		return "", err
	}

	return filepath.Join(detentDir, "repos", repoID+".db"), nil
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
	// Get the new consolidated database path: ~/.detent/repos/<repoID>.db
	dbPath, err := GetDatabasePath(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to compute database path: %w", err)
	}

	// Create repos directory
	reposDir := filepath.Dir(dbPath)
	if mkdirErr := createDirIfNotExists(reposDir); mkdirErr != nil {
		return nil, fmt.Errorf("failed to create repos directory: %w", mkdirErr)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Configure connection pool for CLI usage (single connection is optimal for SQLite)
	db.SetMaxOpenConns(1)    // SQLite performs best with single writer
	db.SetMaxIdleConns(1)    // Keep connection warm for subsequent queries
	db.SetConnMaxLifetime(0) // Don't close idle connections (CLI is short-lived)

	// Apply performance pragmas for 2-5x speedup
	pragmas := []string{
		"PRAGMA journal_mode=WAL",          // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous=NORMAL",        // Faster writes, still safe with WAL
		"PRAGMA cache_size=-64000",         // 64MB cache for better performance
		"PRAGMA busy_timeout=5000",         // Wait 5s on lock instead of failing immediately
		"PRAGMA mmap_size=268435456",       // 256MB memory-mapped I/O for faster reads
		"PRAGMA temp_store=MEMORY",         // Store temp tables in memory
		"PRAGMA page_size=4096",            // Optimal page size for most filesystems
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

	// Set secure file permissions on database and related files (owner read/write only)
	// This must be done after schema initialization which creates the files
	// SQLite WAL mode creates additional files: .db-wal and .db-shm
	if err := secureDBFiles(dbPath); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to set database permissions: %w (additionally, failed to close database: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to set database permissions: %w", err)
	}

	return writer, nil
}

// secureDBFiles sets restrictive permissions (0600) on the database file and
// associated WAL/SHM files created by SQLite in WAL mode.
func secureDBFiles(dbPath string) error {
	// Main database file
	// #nosec G302 - intentionally setting restrictive permissions
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", dbPath, err)
	}

	// WAL and SHM files (may not exist yet, ignore ENOENT)
	walFiles := []string{dbPath + "-wal", dbPath + "-shm"}
	for _, f := range walFiles {
		// #nosec G302 - intentionally setting restrictive permissions
		if err := os.Chmod(f, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("chmod %s: %w", f, err)
		}
	}

	return nil
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
		{
			version: 9,
			name:    "add_file_hash_to_heals",
			sql: `
			ALTER TABLE heals ADD COLUMN file_hash TEXT;
			CREATE INDEX IF NOT EXISTS idx_heals_file_hash ON heals(file_hash);
			`,
		},
		{
			version: 10,
			name:    "add_composite_index_for_heal_cache_lookup",
			sql: `
			CREATE INDEX IF NOT EXISTS idx_heals_cache_lookup
			ON heals(file_path, file_hash, status, created_at DESC);
			`,
		},
		{
			version: 11,
			name:    "add_tree_hash_to_runs",
			sql: `
			ALTER TABLE runs ADD COLUMN tree_hash TEXT;
			`,
		},
		{
			version: 12,
			name:    "add_commit_sha_index_drop_unused",
			sql: `
			-- Add missing index for GetRunByCommit lookups
			CREATE INDEX IF NOT EXISTS idx_runs_commit_sha ON runs(commit_sha);

			-- Drop unused indices (tree_hash and sync_status never queried)
			DROP INDEX IF EXISTS idx_runs_tree_hash;
			DROP INDEX IF EXISTS idx_runs_sync_status;
			DROP INDEX IF EXISTS idx_heals_sync_status;

			-- idx_heals_file_hash is redundant with idx_heals_cache_lookup composite
			DROP INDEX IF EXISTS idx_heals_file_hash;
			`,
		},
		{
			version: 13,
			name:    "add_ai_troubleshooting_and_compliance_fields",
			sql: `
			-- AI troubleshooting fields (used by heal prompts)
			ALTER TABLE errors ADD COLUMN column_number INTEGER;
			ALTER TABLE errors ADD COLUMN severity TEXT;
			ALTER TABLE errors ADD COLUMN rule_id TEXT;
			ALTER TABLE errors ADD COLUMN source TEXT;
			ALTER TABLE errors ADD COLUMN workflow_job TEXT;

			-- Compliance/debugging field (original CI output)
			ALTER TABLE errors ADD COLUMN raw TEXT;

			-- Indices for common queries
			CREATE INDEX IF NOT EXISTS idx_errors_severity ON errors(severity);
			CREATE INDEX IF NOT EXISTS idx_errors_source ON errors(source);
			`,
		},
		{
			version: 14,
			name:    "add_spend_log",
			sql: `
			CREATE TABLE IF NOT EXISTS spend_log (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				cost_usd REAL NOT NULL,
				created_at INTEGER NOT NULL
			);

			CREATE INDEX IF NOT EXISTS idx_spend_log_created_at ON spend_log(created_at);
			`,
		},
		{
			version: 15,
			name:    "add_heals_error_id_attempt_index",
			sql: `
			CREATE INDEX IF NOT EXISTS idx_heals_error_id_attempt ON heals(error_id, attempt_number);
			`,
		},
		{
			version: 16,
			name:    "add_run_errors_junction",
			sql: `
			-- Junction table to track which errors appeared in which runs
			-- This fixes caching: errors were deduplicated across runs but stored with only the first run_id
			CREATE TABLE IF NOT EXISTS run_errors (
				run_id TEXT NOT NULL,
				error_id TEXT NOT NULL,
				PRIMARY KEY (run_id, error_id),
				FOREIGN KEY (run_id) REFERENCES runs(run_id),
				FOREIGN KEY (error_id) REFERENCES errors(error_id)
			);

			CREATE INDEX IF NOT EXISTS idx_run_errors_run_id ON run_errors(run_id);
			CREATE INDEX IF NOT EXISTS idx_run_errors_error_id ON run_errors(error_id);

			-- Backfill existing data: link errors to their original run_id
			INSERT OR IGNORE INTO run_errors (run_id, error_id)
			SELECT run_id, error_id FROM errors WHERE run_id IS NOT NULL;
			`,
		},
		{
			version: 17,
			name:    "add_gc_indices",
			sql: `
			-- Index for GC queries filtering runs by completed_at
			CREATE INDEX IF NOT EXISTS idx_runs_completed_at ON runs(completed_at);

			-- Index for GC orphan error detection (status + last_seen_at filter)
			CREATE INDEX IF NOT EXISTS idx_errors_status_last_seen ON errors(status, last_seen_at);
			`,
		},
		{
			version: 18,
			name:    "add_heal_locks_table",
			sql: `
			-- Advisory lock table for preventing concurrent heal processes on the same repo
			CREATE TABLE IF NOT EXISTS heal_locks (
				repo_path TEXT PRIMARY KEY,
				holder_id TEXT NOT NULL,
				acquired_at INTEGER NOT NULL,
				expires_at INTEGER NOT NULL,
				pid INTEGER
			);
			CREATE INDEX IF NOT EXISTS idx_heal_locks_expires_at ON heal_locks(expires_at);
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
func (w *SQLiteWriter) RecordRun(runID, workflowName, commitSHA, treeHash, executionMode string) error {
	// Note: is_dirty, dirty_files, base_commit_sha, codebase_state_hash are deprecated
	// We now require clean commits before running, so is_dirty is always 0
	query := `
		INSERT INTO runs (run_id, workflow_name, commit_sha, tree_hash, execution_mode, started_at, is_dirty)
		VALUES (?, ?, ?, ?, ?, ?, 0)
	`

	_, err := w.db.Exec(query, runID, workflowName, commitSHA, treeHash, executionMode, time.Now().Unix())
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
	ErrorID      string
	RunID        string
	FilePath     string
	LineNumber   int
	ColumnNumber int
	ErrorType    string
	Message      string
	StackTrace   string
	FileHash     string
	ContentHash  string
	Severity     string
	RuleID       string
	Source       string
	WorkflowJob  string
	Raw          string
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
	SeenCount    int
	Status       string
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

// RunExists checks if a run with the given ID exists in the database.
// This is more efficient than GetRunByID when you only need to check existence.
func (w *SQLiteWriter) RunExists(runID string) (bool, error) {
	var exists int
	err := w.db.QueryRow("SELECT 1 FROM runs WHERE run_id = ? LIMIT 1", runID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check run existence: %w", err)
	}
	return true, nil
}

// GetErrorsByRunID retrieves all errors for a given run via the run_errors junction table.
// This ensures we get ALL errors that appeared in the run, including deduplicated ones
// that were first seen in previous runs.
func (w *SQLiteWriter) GetErrorsByRunID(runID string) ([]*ErrorRecord, error) {
	query := `
		SELECT e.error_id, e.run_id, e.file_path, e.line_number, e.column_number,
			e.error_type, e.message, e.stack_trace, e.file_hash, e.content_hash,
			e.severity, e.rule_id, e.source, e.workflow_job, e.raw,
			e.first_seen_at, e.last_seen_at, e.seen_count, e.status
		FROM errors e
		INNER JOIN run_errors re ON e.error_id = re.error_id
		WHERE re.run_id = ?
		ORDER BY e.file_path, e.line_number
	`

	rows, err := w.db.Query(query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query errors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []*ErrorRecord
	for rows.Next() {
		var e ErrorRecord
		var filePath, stackTrace, fileHash, contentHash sql.NullString
		var severity, ruleID, source, workflowJob, raw, status sql.NullString
		var columnNumber sql.NullInt64
		var firstSeenAt, lastSeenAt sql.NullInt64

		err := rows.Scan(
			&e.ErrorID,
			&e.RunID,
			&filePath,
			&e.LineNumber,
			&columnNumber,
			&e.ErrorType,
			&e.Message,
			&stackTrace,
			&fileHash,
			&contentHash,
			&severity,
			&ruleID,
			&source,
			&workflowJob,
			&raw,
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
		e.Severity = severity.String
		e.RuleID = ruleID.String
		e.Source = source.String
		e.WorkflowJob = workflowJob.String
		e.Raw = raw.String
		e.Status = status.String
		if columnNumber.Valid {
			e.ColumnNumber = int(columnNumber.Int64)
		}
		if firstSeenAt.Valid {
			e.FirstSeenAt = time.Unix(firstSeenAt.Int64, 0)
		}
		if lastSeenAt.Valid {
			e.LastSeenAt = time.Unix(lastSeenAt.Int64, 0)
		}

		records = append(records, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating errors: %w", err)
	}

	return records, nil
}

// RecordFindings adds multiple findings in a single batch operation
func (w *SQLiteWriter) RecordFindings(findings []*FindingRecord) error {
	if len(findings) == 0 {
		return nil
	}

	w.batchMutex.Lock()
	defer w.batchMutex.Unlock()

	// Pre-allocate if batch is empty and findings will exceed capacity
	if len(w.batch) == 0 && len(findings) > cap(w.batch) {
		w.batch = make([]*FindingRecord, 0, len(findings))
	}

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

// Flush forces any pending batched errors to be written to the database
func (w *SQLiteWriter) Flush() error {
	w.batchMutex.Lock()
	defer w.batchMutex.Unlock()
	return w.flushBatch()
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
	var insertStmt, updateStmt, runErrorStmt *sql.Stmt
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
		if runErrorStmt != nil {
			if closeErr := runErrorStmt.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("failed to close run_errors statement: %w", closeErr)
			}
		}
	}()

	insertStmt, err = tx.Prepare(`
		INSERT INTO errors (
			error_id, run_id, file_path, line_number, column_number,
			error_type, message, stack_trace, file_hash, content_hash,
			severity, rule_id, source, workflow_job, raw,
			first_seen_at, last_seen_at, seen_count, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'open')
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

	// Prepare statement for run_errors junction table (links errors to runs for proper caching)
	runErrorStmt, err = tx.Prepare(`INSERT OR IGNORE INTO run_errors (run_id, error_id) VALUES (?, ?)`)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("failed to prepare run_errors statement: %w (additionally, failed to rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("failed to prepare run_errors statement: %w", err)
	}

	now := time.Now().Unix()

	// Compute content hashes once per finding (use pre-computed if available)
	findingHashes := make(map[*FindingRecord]string, len(w.batch))
	contentHashes := make([]string, 0, len(w.batch))
	seenHashes := make(map[string]bool)

	for _, finding := range w.batch {
		// Use pre-computed hash if available, otherwise compute
		contentHash := finding.ContentHash
		if contentHash == "" {
			contentHash = ComputeContentHash(finding.Message)
		}
		findingHashes[finding] = contentHash
		if !seenHashes[contentHash] {
			contentHashes = append(contentHashes, contentHash)
			seenHashes[contentHash] = true
		}
	}

	// Batch lookup existing errors using WHERE IN clause
	existingErrors := make(map[string]string) // content_hash -> error_id

	if len(contentHashes) > 0 {
		// Build query with placeholders for IN clause
		placeholders := make([]string, len(contentHashes))
		args := make([]any, len(contentHashes))
		for i, hash := range contentHashes {
			placeholders[i] = "?"
			args[i] = hash
		}

		// #nosec G201 - SQL string formatting with placeholders only (not user data)
		query := fmt.Sprintf(`
			SELECT content_hash, error_id
			FROM errors
			WHERE content_hash IN (%s)
		`, strings.Join(placeholders, ","))

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
		// Check for errors that occurred during iteration
		if rowsErr := rows.Err(); rowsErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return fmt.Errorf("error iterating existing errors: %w (additionally, failed to close rows: %v)", rowsErr, closeErr)
			}
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("error iterating existing errors: %w (additionally, failed to rollback: %v)", rowsErr, rbErr)
			}
			return fmt.Errorf("error iterating existing errors: %w", rowsErr)
		}
		if closeErr := rows.Close(); closeErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to close rows: %w (additionally, failed to rollback: %v)", closeErr, rbErr)
			}
			return fmt.Errorf("failed to close rows: %w", closeErr)
		}
	}

	// Process all findings using cached hashes (avoid recomputing)
	for _, finding := range w.batch {
		contentHash := findingHashes[finding]
		var errorID string

		if existingID, exists := existingErrors[contentHash]; exists {
			// Update existing error
			errorID = existingID
			_, updateErr := updateStmt.Exec(now, existingID)
			if updateErr != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					return fmt.Errorf("failed to update error: %w (additionally, failed to rollback: %v)", updateErr, rbErr)
				}
				return fmt.Errorf("failed to update error: %w", updateErr)
			}
		} else {
			// Insert new error
			var uuidErr error
			errorID, uuidErr = util.GenerateUUID()
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
				finding.Column,
				finding.Category,
				finding.Message,
				finding.StackTrace,
				finding.FileHash,
				contentHash,
				finding.Severity,
				finding.RuleID,
				finding.Source,
				finding.WorkflowJob,
				finding.Raw,
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

		// Link error to current run in junction table (enables proper cache retrieval)
		_, runErrExecErr := runErrorStmt.Exec(finding.RunID, errorID)
		if runErrExecErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to insert run_error: %w (additionally, failed to rollback: %v)", runErrExecErr, rbErr)
			}
			return fmt.Errorf("failed to insert run_error: %w", runErrExecErr)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	w.batch = w.batch[:0]
	return nil
}

// FlushBatch flushes any remaining batched errors (public method for external callers).
//
// Deprecated: Use Flush() instead. This is kept for backwards compatibility.
func (w *SQLiteWriter) FlushBatch() error {
	return w.Flush()
}

// FinalizeRun updates the run record with completion information.
// Note: Caller should call Flush() before this method to ensure accurate error counts.
func (w *SQLiteWriter) FinalizeRun(runID string, totalErrors int) error {
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
			heal_id, error_id, run_id, diff_content, diff_content_hash, file_path, file_hash,
			model_id, prompt_hash, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens, cost_usd,
			status, created_at, applied_at, verified_at, verification_result,
			attempt_number, parent_heal_id, failure_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		heal.FileHash,
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
	var args []any

	if appliedAt != nil {
		query = `UPDATE heals SET status = ?, applied_at = ? WHERE heal_id = ?`
		args = []any{string(status), appliedAt.Unix(), healID}
	} else {
		query = `UPDATE heals SET status = ? WHERE heal_id = ?`
		args = []any{string(status), healID}
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
	query := `SELECT ` + healSelectColumns + ` FROM heals WHERE error_id = ? ORDER BY attempt_number ASC`

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
	query := `SELECT ` + healSelectColumns + ` FROM heals WHERE error_id = ? ORDER BY attempt_number DESC LIMIT 1`

	row := w.db.QueryRow(query, errorID)
	heal, err := scanHealRowSingle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest heal: %w", err)
	}

	return heal, nil
}

// GetPendingHealByFileHash finds a reusable pending heal for the given file and hash
func (w *SQLiteWriter) GetPendingHealByFileHash(filePath, fileHash string) (*HealRecord, error) {
	query := `SELECT ` + healSelectColumns + ` FROM heals
		WHERE file_path = ? AND file_hash = ? AND status = 'pending'
		ORDER BY created_at DESC LIMIT 1`

	row := w.db.QueryRow(query, filePath, fileHash)
	heal, err := scanHealRowSingle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get pending heal: %w", err)
	}

	return heal, nil
}

// healScanner abstracts sql.Row and sql.Rows for shared scanning logic
type healScanner interface {
	Scan(dest ...any) error
}

// scanHealFromScanner scans a heal record from any scanner (sql.Row or sql.Rows)
func scanHealFromScanner(s healScanner) (*HealRecord, error) {
	var heal HealRecord
	var runID, diffContentHash, filePath, fileHash, modelID, promptHash sql.NullString
	var appliedAt, verifiedAt sql.NullInt64
	var verificationResult, parentHealID, failureReason sql.NullString
	var createdAtUnix int64
	var status string

	err := s.Scan(
		&heal.HealID,
		&heal.ErrorID,
		&runID,
		&heal.DiffContent,
		&diffContentHash,
		&filePath,
		&fileHash,
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
	heal.FileHash = fileHash.String
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

// scanHealRow scans a heal row from sql.Rows
func scanHealRow(rows *sql.Rows) (*HealRecord, error) {
	return scanHealFromScanner(rows)
}

// scanHealRowSingle scans a heal row from sql.Row
func scanHealRowSingle(row *sql.Row) (*HealRecord, error) {
	return scanHealFromScanner(row)
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

// Close closes the database connection and secures file permissions.
func (w *SQLiteWriter) Close() error {
	if w.db == nil {
		return nil
	}

	// Close the database first
	closeErr := w.db.Close()

	// Secure WAL/SHM files that may have been created during usage
	// Do this even if close failed (best effort)
	if w.path != "" {
		_ = secureDBFiles(w.path)
	}

	return closeErr
}

// Path returns the absolute path to the SQLite database file
func (w *SQLiteWriter) Path() string {
	return w.path
}

// --- Heal Locks ---

// AcquireHealLock attempts to acquire an exclusive heal lock for a repository.
// Returns the holder ID on success, or ErrHealLockHeld if the lock is already held.
// The lock automatically expires after the specified timeout for crash recovery.
func (w *SQLiteWriter) AcquireHealLock(repoPath string, timeout time.Duration) (string, error) {
	now := time.Now()
	expiresAt := now.Add(timeout)

	// Generate unique holder ID
	holderID, err := util.GenerateUUID()
	if err != nil {
		return "", fmt.Errorf("failed to generate holder ID: %w", err)
	}

	// Get current process ID for debugging
	pid := os.Getpid()

	// Start a transaction to ensure atomicity
	tx, err := w.db.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // No-op if committed

	// First, clean up any expired locks
	_, err = tx.Exec("DELETE FROM heal_locks WHERE expires_at < ?", now.Unix())
	if err != nil {
		return "", fmt.Errorf("failed to clean expired locks: %w", err)
	}

	// Try to insert the lock (will fail if lock exists due to PRIMARY KEY constraint)
	_, err = tx.Exec(
		"INSERT INTO heal_locks (repo_path, holder_id, acquired_at, expires_at, pid) VALUES (?, ?, ?, ?, ?)",
		repoPath, holderID, now.Unix(), expiresAt.Unix(), pid,
	)
	if err != nil {
		// Check if this is a constraint violation (lock already held)
		if strings.Contains(err.Error(), "UNIQUE constraint failed") ||
			strings.Contains(err.Error(), "PRIMARY KEY constraint failed") {
			// Get info about who holds the lock
			var existingHolder string
			var existingPID sql.NullInt64
			_ = tx.QueryRow(
				"SELECT holder_id, pid FROM heal_locks WHERE repo_path = ?",
				repoPath,
			).Scan(&existingHolder, &existingPID)

			pidInfo := ""
			if existingPID.Valid {
				pidInfo = fmt.Sprintf(" (pid: %d)", existingPID.Int64)
			}
			return "", fmt.Errorf("%w: held by %s%s", ErrHealLockHeld, existingHolder[:8], pidInfo)
		}
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit lock: %w", err)
	}

	return holderID, nil
}

// ReleaseHealLock releases a previously acquired heal lock.
// Only the holder (matching holderID) can release the lock.
// This operation is idempotent - releasing a non-existent lock is not an error.
func (w *SQLiteWriter) ReleaseHealLock(repoPath, holderID string) error {
	result, err := w.db.Exec(
		"DELETE FROM heal_locks WHERE repo_path = ? AND holder_id = ?",
		repoPath, holderID,
	)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	// Log if lock was already released (not an error, but useful for debugging)
	if rowsAffected == 0 {
		// Lock was already released or expired - this is fine
		return nil
	}

	return nil
}

// IsHealLockHeld checks if a valid (non-expired) heal lock exists for a repository.
// Returns true if locked, along with the holder ID.
func (w *SQLiteWriter) IsHealLockHeld(repoPath string) (bool, string, error) {
	now := time.Now().Unix()

	var holderID string
	err := w.db.QueryRow(
		"SELECT holder_id FROM heal_locks WHERE repo_path = ? AND expires_at > ?",
		repoPath, now,
	).Scan(&holderID)

	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to check lock status: %w", err)
	}

	return true, holderID, nil
}

// --- Garbage Collection ---

// GCStats holds the results of a garbage collection operation.
type GCStats struct {
	RunsDeleted           int
	RunErrorsDeleted      int
	ErrorLocationsDeleted int
	HealsDeleted          int
	ErrorsDeleted         int
	DryRun                bool
}

// GarbageCollect removes old data based on retention policy.
// Deletes runs older than retentionDays along with associated data.
// Preserves heals with status='applied' for audit trail.
// Preserves errors with status='open' regardless of age.
// Returns stats about what was (or would be in dry-run mode) deleted.
func (w *SQLiteWriter) GarbageCollect(retentionDays int, dryRun bool) (*GCStats, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	if dryRun {
		return w.countGCTargets(cutoff)
	}

	stats := &GCStats{DryRun: false}

	tx, err := w.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin gc transaction: %w", err)
	}

	// Rollback on any error (commit will clear this)
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 1. Delete run_errors for expired runs
	result, err := tx.Exec(`
		DELETE FROM run_errors
		WHERE run_id IN (
			SELECT run_id FROM runs
			WHERE completed_at IS NOT NULL AND completed_at < ?
		)`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to delete run_errors: %w", err)
	}
	affected, _ := result.RowsAffected()
	stats.RunErrorsDeleted = int(affected)

	// 2. Delete error_locations for expired runs
	result, err = tx.Exec(`
		DELETE FROM error_locations
		WHERE run_id IN (
			SELECT run_id FROM runs
			WHERE completed_at IS NOT NULL AND completed_at < ?
		)`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to delete error_locations: %w", err)
	}
	affected, _ = result.RowsAffected()
	stats.ErrorLocationsDeleted = int(affected)

	// 3. Delete heals for expired runs and orphaned errors (preserve applied status for audit)
	result, err = tx.Exec(`
		DELETE FROM heals
		WHERE status != 'applied'
		AND (
			run_id IN (
				SELECT run_id FROM runs
				WHERE completed_at IS NOT NULL AND completed_at < ?
			)
			OR (run_id IS NULL AND created_at < ?)
			OR error_id IN (
				SELECT e.error_id FROM errors e
				WHERE e.status != 'open'
				  AND e.last_seen_at < ?
				  AND NOT EXISTS (
					SELECT 1 FROM run_errors re
					WHERE re.error_id = e.error_id
					  AND re.run_id IN (
						  SELECT run_id FROM runs
						  WHERE completed_at IS NULL OR completed_at >= ?
					  )
				  )
			)
		)`, cutoff, cutoff, cutoff, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to delete heals: %w", err)
	}
	affected, _ = result.RowsAffected()
	stats.HealsDeleted = int(affected)

	// 4. Delete orphaned errors (not 'open', not seen recently, not linked to remaining runs)
	result, err = tx.Exec(`
		DELETE FROM errors
		WHERE error_id IN (
			SELECT e.error_id FROM errors e
			WHERE e.status != 'open'
			  AND e.last_seen_at < ?
			  AND NOT EXISTS (
				SELECT 1 FROM run_errors re
				WHERE re.error_id = e.error_id
				  AND re.run_id IN (
					  SELECT run_id FROM runs
					  WHERE completed_at IS NULL OR completed_at >= ?
				  )
			  )
		)`, cutoff, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to delete orphaned errors: %w", err)
	}
	affected, _ = result.RowsAffected()
	stats.ErrorsDeleted = int(affected)

	// 5. Delete expired runs
	result, err = tx.Exec(`
		DELETE FROM runs
		WHERE completed_at IS NOT NULL AND completed_at < ?`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to delete runs: %w", err)
	}
	affected, _ = result.RowsAffected()
	stats.RunsDeleted = int(affected)

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit gc transaction: %w", err)
	}
	committed = true

	return stats, nil
}

// countGCTargets counts what would be deleted without actually deleting (for dry-run).
func (w *SQLiteWriter) countGCTargets(cutoff int64) (*GCStats, error) {
	stats := &GCStats{DryRun: true}
	var err error

	// Count runs to delete
	err = w.db.QueryRow(`
		SELECT COUNT(*) FROM runs
		WHERE completed_at IS NOT NULL AND completed_at < ?`, cutoff).Scan(&stats.RunsDeleted)
	if err != nil {
		return nil, fmt.Errorf("failed to count runs: %w", err)
	}

	// Count run_errors to delete
	err = w.db.QueryRow(`
		SELECT COUNT(*) FROM run_errors
		WHERE run_id IN (
			SELECT run_id FROM runs
			WHERE completed_at IS NOT NULL AND completed_at < ?
		)`, cutoff).Scan(&stats.RunErrorsDeleted)
	if err != nil {
		return nil, fmt.Errorf("failed to count run_errors: %w", err)
	}

	// Count error_locations to delete
	err = w.db.QueryRow(`
		SELECT COUNT(*) FROM error_locations
		WHERE run_id IN (
			SELECT run_id FROM runs
			WHERE completed_at IS NOT NULL AND completed_at < ?
		)`, cutoff).Scan(&stats.ErrorLocationsDeleted)
	if err != nil {
		return nil, fmt.Errorf("failed to count error_locations: %w", err)
	}

	// Count heals to delete (exclude applied)
	err = w.db.QueryRow(`
		SELECT COUNT(*) FROM heals
		WHERE status != 'applied'
		AND (
			run_id IN (
				SELECT run_id FROM runs
				WHERE completed_at IS NOT NULL AND completed_at < ?
			)
			OR (run_id IS NULL AND created_at < ?)
			OR error_id IN (
				SELECT e.error_id FROM errors e
				WHERE e.status != 'open'
				  AND e.last_seen_at < ?
				  AND NOT EXISTS (
					SELECT 1 FROM run_errors re
					WHERE re.error_id = e.error_id
					  AND re.run_id IN (
						  SELECT run_id FROM runs
						  WHERE completed_at IS NULL OR completed_at >= ?
					  )
				  )
			)
		)`, cutoff, cutoff, cutoff, cutoff).Scan(&stats.HealsDeleted)
	if err != nil {
		return nil, fmt.Errorf("failed to count heals: %w", err)
	}

	// Count orphaned errors to delete
	err = w.db.QueryRow(`
		SELECT COUNT(*) FROM errors e
		WHERE e.status != 'open'
		  AND e.last_seen_at < ?
		  AND NOT EXISTS (
			SELECT 1 FROM run_errors re
			WHERE re.error_id = e.error_id
			  AND re.run_id IN (
				  SELECT run_id FROM runs
				  WHERE completed_at IS NULL OR completed_at >= ?
			  )
		  )`, cutoff, cutoff).Scan(&stats.ErrorsDeleted)
	if err != nil {
		return nil, fmt.Errorf("failed to count errors: %w", err)
	}

	return stats, nil
}

// ListRepoDatabases returns all database files in ~/.detent/repos/.
func ListRepoDatabases() ([]string, error) {
	detentDir, err := GetDetentDir()
	if err != nil {
		return nil, err
	}

	reposDir := filepath.Join(detentDir, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read repos directory: %w", err)
	}

	var dbs []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".db") {
			dbs = append(dbs, filepath.Join(reposDir, entry.Name()))
		}
	}
	return dbs, nil
}

// OpenDatabaseDirect opens a database file directly without requiring a repo path.
// Used by the gc command to iterate over all databases in ~/.detent/repos/.
// Runs migrations to ensure schema is up-to-date before GC operations.
// For security, only allows opening .db files within ~/.detent/repos/.
func OpenDatabaseDirect(dbPath string) (*SQLiteWriter, error) {
	// Validate path is within ~/.detent/repos/ to prevent path traversal
	detentDir, err := GetDetentDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get detent directory: %w", err)
	}

	reposDir := filepath.Join(detentDir, "repos")
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Clean both paths and ensure absPath is within reposDir
	cleanReposDir := filepath.Clean(reposDir) + string(filepath.Separator)
	cleanAbsPath := filepath.Clean(absPath)
	if !strings.HasPrefix(cleanAbsPath, cleanReposDir) {
		return nil, fmt.Errorf("path outside repos directory")
	}

	// Ensure it's a .db file (not a directory or other file type)
	if !strings.HasSuffix(cleanAbsPath, ".db") {
		return nil, fmt.Errorf("not a database file")
	}

	db, err := sql.Open("sqlite3", cleanAbsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Apply essential pragmas
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, pragmaErr := db.Exec(pragma); pragmaErr != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to execute %s: %w", pragma, pragmaErr)
		}
	}

	writer := &SQLiteWriter{db: db, path: cleanAbsPath}

	// Run migrations to ensure schema is up-to-date (required for GC queries)
	if err := writer.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return writer, nil
}
