// TODO: Add Drizzle ORM
// TODO: Configure PlanetScale connection
// TODO: Add migrations

// Placeholder schema for future PlanetScale integration
// Will use Drizzle ORM for type-safe queries

/*
import { mysqlTable, varchar, text, timestamp, json, int } from "drizzle-orm/mysql-core";

export const organizations = mysqlTable("organizations", {
  id: varchar("id", { length: 36 }).primaryKey(),
  name: varchar("name", { length: 255 }).notNull(),
  workosOrgId: varchar("workos_org_id", { length: 255 }),
  createdAt: timestamp("created_at").defaultNow().notNull(),
});

export const apiKeys = mysqlTable("api_keys", {
  id: varchar("id", { length: 36 }).primaryKey(),
  organizationId: varchar("organization_id", { length: 36 }).notNull(),
  keyHash: varchar("key_hash", { length: 255 }).notNull(),
  name: varchar("name", { length: 255 }),
  lastUsedAt: timestamp("last_used_at"),
  createdAt: timestamp("created_at").defaultNow().notNull(),
});

export const repositories = mysqlTable("repositories", {
  id: varchar("id", { length: 36 }).primaryKey(),
  organizationId: varchar("organization_id", { length: 36 }).notNull(),
  githubRepoId: varchar("github_repo_id", { length: 255 }).notNull(),
  fullName: varchar("full_name", { length: 255 }).notNull(),
  defaultBranch: varchar("default_branch", { length: 255 }).default("main"),
  createdAt: timestamp("created_at").defaultNow().notNull(),
});

export const runs = mysqlTable("runs", {
  id: varchar("id", { length: 36 }).primaryKey(),
  repositoryId: varchar("repository_id", { length: 36 }).notNull(),
  prNumber: int("pr_number"),
  commitSha: varchar("commit_sha", { length: 40 }).notNull(),
  status: varchar("status", { length: 50 }).notNull(), // pending, parsing, healing, completed, failed
  errorsJson: json("errors_json"),
  createdAt: timestamp("created_at").defaultNow().notNull(),
  completedAt: timestamp("completed_at"),
});

export const patches = mysqlTable("patches", {
  id: varchar("id", { length: 36 }).primaryKey(),
  runId: varchar("run_id", { length: 36 }).notNull(),
  filePath: varchar("file_path", { length: 1024 }).notNull(),
  diff: text("diff").notNull(),
  applied: int("applied").default(0), // boolean
  createdAt: timestamp("created_at").defaultNow().notNull(),
});
*/

export {};
