/**
 * Database schema and migrations for Detent persistence
 * Ported from Go: apps/go-cli/internal/persistence/sqlite.go
 *
 * Schema version: 19
 * Tables: runs, errors, run_errors, heals, error_locations,
 *         assignments, suggested_fixes, fix_errors,
 *         spend_log, heal_locks, schema_version
 */

import type Database from "better-sqlite3";

export const CURRENT_SCHEMA_VERSION = 19;

interface Migration {
  version: number;
  name: string;
  sql: string;
}

/**
 * All database migrations from version 1 to current
 */
export const migrations: Migration[] = [
  {
    version: 1,
    name: "initial_schema",
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
    name: "add_worktree_tracking",
    sql: `
      ALTER TABLE runs ADD COLUMN is_dirty INTEGER DEFAULT 0;
      ALTER TABLE runs ADD COLUMN dirty_files TEXT;
      ALTER TABLE runs ADD COLUMN base_commit_sha TEXT;

      CREATE INDEX IF NOT EXISTS idx_runs_is_dirty ON runs(is_dirty);
    `,
  },
  {
    version: 3,
    name: "add_heals_table",
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
    name: "add_error_locations",
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
    name: "add_sync_and_state_tracking",
    sql: `
      ALTER TABLE runs ADD COLUMN codebase_state_hash TEXT;
      ALTER TABLE runs ADD COLUMN sync_status TEXT DEFAULT 'pending';
      ALTER TABLE errors ADD COLUMN sync_status TEXT DEFAULT 'pending';
      ALTER TABLE heals ADD COLUMN sync_status TEXT DEFAULT 'pending';

      CREATE INDEX IF NOT EXISTS idx_runs_codebase_state_hash ON runs(codebase_state_hash);
      CREATE INDEX IF NOT EXISTS idx_runs_sync_status ON runs(sync_status);
      CREATE INDEX IF NOT EXISTS idx_errors_sync_status ON errors(sync_status);
      CREATE INDEX IF NOT EXISTS idx_heals_sync_status ON heals(sync_status);
    `,
  },
  {
    version: 6,
    name: "drop_unused_indices",
    sql: `
      DROP INDEX IF EXISTS idx_errors_run_id;
      DROP INDEX IF EXISTS idx_errors_status;
      DROP INDEX IF EXISTS idx_errors_content_hash_time;
      DROP INDEX IF EXISTS idx_runs_is_dirty;
      DROP INDEX IF EXISTS idx_errors_sync_status;
    `,
  },
  {
    version: 7,
    name: "drop_dirty_state_tracking",
    sql: `
      DROP INDEX IF EXISTS idx_runs_codebase_state_hash;
    `,
  },
  {
    version: 8,
    name: "restore_run_id_index_for_cache",
    sql: `
      CREATE INDEX IF NOT EXISTS idx_errors_run_id ON errors(run_id);
    `,
  },
  {
    version: 9,
    name: "add_file_hash_to_heals",
    sql: `
      ALTER TABLE heals ADD COLUMN file_hash TEXT;
      CREATE INDEX IF NOT EXISTS idx_heals_file_hash ON heals(file_hash);
    `,
  },
  {
    version: 10,
    name: "add_composite_index_for_heal_cache_lookup",
    sql: `
      CREATE INDEX IF NOT EXISTS idx_heals_cache_lookup
      ON heals(file_path, file_hash, status, created_at DESC);
    `,
  },
  {
    version: 11,
    name: "add_tree_hash_to_runs",
    sql: `
      ALTER TABLE runs ADD COLUMN tree_hash TEXT;
    `,
  },
  {
    version: 12,
    name: "add_commit_sha_index_drop_unused",
    sql: `
      CREATE INDEX IF NOT EXISTS idx_runs_commit_sha ON runs(commit_sha);
      DROP INDEX IF EXISTS idx_runs_tree_hash;
      DROP INDEX IF EXISTS idx_runs_sync_status;
      DROP INDEX IF EXISTS idx_heals_sync_status;
      DROP INDEX IF EXISTS idx_heals_file_hash;
    `,
  },
  {
    version: 13,
    name: "add_ai_troubleshooting_and_compliance_fields",
    sql: `
      ALTER TABLE errors ADD COLUMN column_number INTEGER;
      ALTER TABLE errors ADD COLUMN severity TEXT;
      ALTER TABLE errors ADD COLUMN rule_id TEXT;
      ALTER TABLE errors ADD COLUMN source TEXT;
      ALTER TABLE errors ADD COLUMN workflow_job TEXT;
      ALTER TABLE errors ADD COLUMN raw TEXT;

      CREATE INDEX IF NOT EXISTS idx_errors_severity ON errors(severity);
      CREATE INDEX IF NOT EXISTS idx_errors_source ON errors(source);
    `,
  },
  {
    version: 14,
    name: "add_spend_log",
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
    name: "add_heals_error_id_attempt_index",
    sql: `
      CREATE INDEX IF NOT EXISTS idx_heals_error_id_attempt ON heals(error_id, attempt_number);
    `,
  },
  {
    version: 16,
    name: "add_run_errors_junction",
    sql: `
      CREATE TABLE IF NOT EXISTS run_errors (
        run_id TEXT NOT NULL,
        error_id TEXT NOT NULL,
        PRIMARY KEY (run_id, error_id),
        FOREIGN KEY (run_id) REFERENCES runs(run_id),
        FOREIGN KEY (error_id) REFERENCES errors(error_id)
      );

      CREATE INDEX IF NOT EXISTS idx_run_errors_run_id ON run_errors(run_id);
      CREATE INDEX IF NOT EXISTS idx_run_errors_error_id ON run_errors(error_id);

      INSERT OR IGNORE INTO run_errors (run_id, error_id)
      SELECT run_id, error_id FROM errors WHERE run_id IS NOT NULL;
    `,
  },
  {
    version: 17,
    name: "add_gc_indices",
    sql: `
      CREATE INDEX IF NOT EXISTS idx_runs_completed_at ON runs(completed_at);
      CREATE INDEX IF NOT EXISTS idx_errors_status_last_seen ON errors(status, last_seen_at);
    `,
  },
  {
    version: 18,
    name: "add_heal_locks_table",
    sql: `
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
  {
    version: 19,
    name: "add_suggested_fixes_and_assignments",
    sql: `
      CREATE TABLE IF NOT EXISTS assignments (
        assignment_id TEXT PRIMARY KEY,
        run_id TEXT NOT NULL,
        agent_id TEXT NOT NULL,
        worktree_path TEXT,
        error_count INTEGER NOT NULL,
        error_ids_json TEXT NOT NULL,
        status TEXT DEFAULT 'assigned',
        created_at INTEGER NOT NULL,
        started_at INTEGER,
        completed_at INTEGER,
        expires_at INTEGER NOT NULL,
        fix_id TEXT,
        failure_reason TEXT,
        FOREIGN KEY (run_id) REFERENCES runs(run_id)
      );

      CREATE INDEX IF NOT EXISTS idx_assignments_run_id ON assignments(run_id);
      CREATE INDEX IF NOT EXISTS idx_assignments_status ON assignments(status);
      CREATE INDEX IF NOT EXISTS idx_assignments_expires ON assignments(expires_at);

      CREATE TABLE IF NOT EXISTS suggested_fixes (
        fix_id TEXT PRIMARY KEY,
        assignment_id TEXT NOT NULL,
        agent_id TEXT,
        worktree_path TEXT,
        file_changes_json TEXT NOT NULL,
        explanation TEXT,
        confidence INTEGER DEFAULT 80,
        verification_command TEXT NOT NULL,
        verification_exit_code INTEGER NOT NULL,
        verification_output TEXT,
        verification_duration_ms INTEGER,
        errors_before INTEGER NOT NULL,
        errors_after INTEGER NOT NULL,
        model_id TEXT,
        input_tokens INTEGER DEFAULT 0,
        output_tokens INTEGER DEFAULT 0,
        cost_usd REAL DEFAULT 0,
        status TEXT DEFAULT 'pending',
        created_at INTEGER NOT NULL,
        applied_at INTEGER,
        applied_by TEXT,
        applied_commit_sha TEXT,
        rejected_at INTEGER,
        rejected_by TEXT,
        rejection_reason TEXT,
        FOREIGN KEY (assignment_id) REFERENCES assignments(assignment_id)
      );

      CREATE INDEX IF NOT EXISTS idx_suggested_fixes_assignment ON suggested_fixes(assignment_id);
      CREATE INDEX IF NOT EXISTS idx_suggested_fixes_status ON suggested_fixes(status);
      CREATE INDEX IF NOT EXISTS idx_suggested_fixes_created ON suggested_fixes(created_at DESC);

      CREATE TABLE IF NOT EXISTS fix_errors (
        fix_id TEXT NOT NULL,
        error_id TEXT NOT NULL,
        PRIMARY KEY (fix_id, error_id),
        FOREIGN KEY (fix_id) REFERENCES suggested_fixes(fix_id),
        FOREIGN KEY (error_id) REFERENCES errors(error_id)
      );

      CREATE INDEX IF NOT EXISTS idx_fix_errors_error_id ON fix_errors(error_id);
    `,
  },
];

/**
 * Initialize the database schema.
 * Creates schema_version table if not exists and applies pending migrations.
 */
export const initSchema = (db: Database.Database): void => {
  // Create schema_version table first
  db.exec(`
    CREATE TABLE IF NOT EXISTS schema_version (
      version INTEGER PRIMARY KEY,
      applied_at INTEGER NOT NULL
    );
  `);

  // Get current schema version
  const row = db
    .prepare("SELECT COALESCE(MAX(version), 0) as version FROM schema_version")
    .get() as { version: number };
  const currentVersion = row.version;

  // Apply migrations if needed
  if (currentVersion < CURRENT_SCHEMA_VERSION) {
    applyMigrations(db, currentVersion);
  }
};

/**
 * Apply database migrations from the current version to the latest
 */
const applyMigrations = (db: Database.Database, fromVersion: number): void => {
  for (const migration of migrations) {
    if (migration.version <= fromVersion) {
      continue;
    }

    // Run migration in a transaction
    const applyMigration = db.transaction(() => {
      // Execute migration SQL (may contain multiple statements)
      db.exec(migration.sql);

      // Record migration in schema_version table
      db.prepare(
        "INSERT INTO schema_version (version, applied_at) VALUES (?, ?)"
      ).run(migration.version, Math.floor(Date.now() / 1000));
    });

    try {
      applyMigration();
    } catch (error) {
      throw new Error(
        `Failed to execute migration v${migration.version} (${migration.name}): ${error}`
      );
    }
  }
};

/**
 * Get the current schema version from the database
 */
export const getSchemaVersion = (db: Database.Database): number => {
  try {
    const row = db
      .prepare(
        "SELECT COALESCE(MAX(version), 0) as version FROM schema_version"
      )
      .get() as { version: number };
    return row.version;
  } catch {
    return 0;
  }
};
