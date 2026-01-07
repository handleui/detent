import { relations } from "drizzle-orm";
import {
  boolean,
  index,
  pgEnum,
  pgTable,
  timestamp,
  uniqueIndex,
  varchar,
} from "drizzle-orm/pg-core";

// ============================================================================
// Enums
// ============================================================================

export const providerEnum = pgEnum("provider", ["github", "gitlab"]);
export const accountTypeEnum = pgEnum("account_type", ["organization", "user"]);
export const teamRoleEnum = pgEnum("team_role", ["owner", "admin", "member"]);

// ============================================================================
// Teams (Detent team ↔ CI Provider Account)
// ============================================================================

export const teams = pgTable(
  "teams",
  {
    id: varchar("id", { length: 36 }).primaryKey(),
    name: varchar("name", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }).notNull().unique(),

    // CI Provider connection (GitHub org/user, GitLab group/user)
    provider: providerEnum("provider").notNull(),
    providerAccountId: varchar("provider_account_id", {
      length: 255,
    }).notNull(),
    providerAccountLogin: varchar("provider_account_login", {
      length: 255,
    }).notNull(),
    providerAccountType: accountTypeEnum("provider_account_type").notNull(),
    providerInstallationId: varchar("provider_installation_id", {
      length: 255,
    })
      .notNull()
      .unique(),
    providerAvatarUrl: varchar("provider_avatar_url", { length: 500 }),

    // Status
    suspendedAt: timestamp("suspended_at"),
    deletedAt: timestamp("deleted_at"),

    // Timestamps
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("teams_slug_idx").on(table.slug),
    uniqueIndex("teams_provider_installation_id_idx").on(
      table.providerInstallationId
    ),
    index("teams_provider_account_idx").on(
      table.provider,
      table.providerAccountId
    ),
  ]
);

// ============================================================================
// Team Members (WorkOS user ↔ Team membership)
// ============================================================================

export const teamMembers = pgTable(
  "team_members",
  {
    id: varchar("id", { length: 36 }).primaryKey(),
    teamId: varchar("team_id", { length: 36 })
      .notNull()
      .references(() => teams.id, { onDelete: "cascade" }),
    userId: varchar("user_id", { length: 255 }).notNull(), // WorkOS user_xxx ID
    role: teamRoleEnum("role").default("member").notNull(),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("team_members_team_user_idx").on(table.teamId, table.userId),
    index("team_members_user_id_idx").on(table.userId),
  ]
);

// ============================================================================
// Projects (Detent project ↔ CI Provider Repo)
// ============================================================================

export const projects = pgTable(
  "projects",
  {
    id: varchar("id", { length: 36 }).primaryKey(),
    teamId: varchar("team_id", { length: 36 })
      .notNull()
      .references(() => teams.id, { onDelete: "cascade" }),

    // CI Provider repo info (GitHub repo, GitLab project)
    providerRepoId: varchar("provider_repo_id", { length: 255 }).notNull(),
    providerRepoName: varchar("provider_repo_name", { length: 255 }).notNull(),
    providerRepoFullName: varchar("provider_repo_full_name", {
      length: 500,
    }).notNull(),
    providerDefaultBranch: varchar("provider_default_branch", { length: 255 }),
    isPrivate: boolean("is_private").default(false).notNull(),

    // Status
    removedAt: timestamp("removed_at"),

    // Timestamps
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    index("projects_team_id_idx").on(table.teamId),
    uniqueIndex("projects_team_repo_idx").on(
      table.teamId,
      table.providerRepoId
    ),
    index("projects_provider_repo_full_name_idx").on(
      table.providerRepoFullName
    ),
  ]
);

// ============================================================================
// Relations (for Drizzle relational query API)
// ============================================================================

export const teamsRelations = relations(teams, ({ many }) => ({
  members: many(teamMembers),
  projects: many(projects),
}));

export const teamMembersRelations = relations(teamMembers, ({ one }) => ({
  team: one(teams, { fields: [teamMembers.teamId], references: [teams.id] }),
}));

export const projectsRelations = relations(projects, ({ one }) => ({
  team: one(teams, { fields: [projects.teamId], references: [teams.id] }),
}));

// ============================================================================
// Type Exports
// ============================================================================

export type Team = typeof teams.$inferSelect;
export type NewTeam = typeof teams.$inferInsert;

export type TeamMember = typeof teamMembers.$inferSelect;
export type NewTeamMember = typeof teamMembers.$inferInsert;

export type Project = typeof projects.$inferSelect;
export type NewProject = typeof projects.$inferInsert;
