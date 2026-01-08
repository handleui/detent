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
export const organizationRoleEnum = pgEnum("organization_role", [
  "owner",
  "admin",
  "member",
]);

// Provider short codes for handles (used in slugs/URLs)
export const providerShortCodes: Record<"github" | "gitlab", string> = {
  github: "gh",
  gitlab: "gl",
};

// Helper to create provider-prefixed slug
export const createProviderSlug = (
  provider: "github" | "gitlab",
  login: string
): string => `${providerShortCodes[provider]}/${login.toLowerCase()}`;

// ============================================================================
// Enterprises (Groups multiple organizations - stub for future use)
// ============================================================================

export const enterprises = pgTable(
  "enterprises",
  {
    id: varchar("id", { length: 36 }).primaryKey(),
    name: varchar("name", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }).notNull().unique(),

    // Status
    suspendedAt: timestamp("suspended_at"),
    deletedAt: timestamp("deleted_at"),

    // Timestamps
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [uniqueIndex("enterprises_slug_idx").on(table.slug)]
);

// ============================================================================
// Organizations (Detent organization ↔ CI Provider Account)
// ============================================================================

export const organizations = pgTable(
  "organizations",
  {
    id: varchar("id", { length: 36 }).primaryKey(),
    name: varchar("name", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }).notNull().unique(),

    // Enterprise grouping (optional, for future use)
    enterpriseId: varchar("enterprise_id", { length: 36 }).references(
      () => enterprises.id,
      { onDelete: "set null" }
    ),

    // CI Provider connection (GitHub org/user, GitLab group/user)
    provider: providerEnum("provider").notNull(),
    providerAccountId: varchar("provider_account_id", {
      length: 255,
    }).notNull(),
    providerAccountLogin: varchar("provider_account_login", {
      length: 255,
    }).notNull(),
    providerAccountType: accountTypeEnum("provider_account_type").notNull(),
    providerAvatarUrl: varchar("provider_avatar_url", { length: 500 }),

    // GitHub-specific: App installation ID (null for GitLab)
    providerInstallationId: varchar("provider_installation_id", {
      length: 255,
    }),

    // GitLab-specific: Encrypted group access token
    providerAccessTokenEncrypted: varchar("provider_access_token_encrypted", {
      length: 500,
    }),
    providerAccessTokenExpiresAt: timestamp("provider_access_token_expires_at"),

    // GitLab-specific: Webhook secret for manual webhook setup
    providerWebhookSecret: varchar("provider_webhook_secret", { length: 255 }),

    // Installer tracking - GitHub ID of user who installed the app (immutable)
    // Used to grant "owner" role to the installer when they first access the org
    installerGithubId: varchar("installer_github_id", { length: 255 }),

    // Status
    suspendedAt: timestamp("suspended_at"),
    deletedAt: timestamp("deleted_at"),

    // Sync tracking - when we last verified state with the provider
    lastSyncedAt: timestamp("last_synced_at"),

    // Timestamps
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("organizations_slug_idx").on(table.slug),
    // GitHub installations: unique when present (null for GitLab orgs)
    // HACK: Drizzle doesn't support partial indexes; actual WHERE NOT NULL
    // constraint is in migration 0003_gitlab_and_enterprise_support.sql
    uniqueIndex("organizations_provider_installation_id_idx").on(
      table.providerInstallationId
    ),
    // Composite unique: same provider account can't be registered twice
    uniqueIndex("organizations_provider_account_unique_idx").on(
      table.provider,
      table.providerAccountId
    ),
    // Enterprise lookup
    index("organizations_enterprise_id_idx").on(table.enterpriseId),
    // Installer lookup (for granting owner role on first access)
    index("organizations_installer_github_id_idx").on(table.installerGithubId),
  ]
);

// ============================================================================
// Organization Members (WorkOS user ↔ Organization membership)
// ============================================================================

export const organizationMembers = pgTable(
  "organization_members",
  {
    id: varchar("id", { length: 36 }).primaryKey(),
    organizationId: varchar("organization_id", { length: 36 })
      .notNull()
      .references(() => organizations.id, { onDelete: "cascade" }),
    userId: varchar("user_id", { length: 255 }).notNull(), // WorkOS user_xxx ID
    role: organizationRoleEnum("role").default("member").notNull(),

    // Provider account linking (GitHub/GitLab via WorkOS OAuth)
    // Provider is inherited from the organization's provider field
    providerUserId: varchar("provider_user_id", { length: 255 }),
    providerUsername: varchar("provider_username", { length: 255 }),
    providerLinkedAt: timestamp("provider_linked_at"),

    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("organization_members_org_user_idx").on(
      table.organizationId,
      table.userId
    ),
    index("organization_members_user_id_idx").on(table.userId),
    index("organization_members_provider_user_id_idx").on(table.providerUserId),
  ]
);

// ============================================================================
// Projects (Detent project ↔ CI Provider Repo)
// ============================================================================

export const projects = pgTable(
  "projects",
  {
    id: varchar("id", { length: 36 }).primaryKey(),
    organizationId: varchar("organization_id", { length: 36 })
      .notNull()
      .references(() => organizations.id, { onDelete: "cascade" }),

    // Handle for URL-friendly identification (e.g., "api" in @gh/handleui/api)
    // Defaults to providerRepoName, unique within organization
    handle: varchar("handle", { length: 255 }).notNull(),

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
    index("projects_organization_id_idx").on(table.organizationId),
    // Handle must be unique within organization
    uniqueIndex("projects_org_handle_idx").on(
      table.organizationId,
      table.handle
    ),
    uniqueIndex("projects_org_repo_idx").on(
      table.organizationId,
      table.providerRepoId
    ),
    index("projects_provider_repo_full_name_idx").on(
      table.providerRepoFullName
    ),
    // Index for webhook lookups by provider repo ID (e.g., repository.renamed events)
    index("projects_provider_repo_id_idx").on(table.providerRepoId),
  ]
);

// ============================================================================
// Relations (for Drizzle relational query API)
// ============================================================================

export const enterprisesRelations = relations(enterprises, ({ many }) => ({
  organizations: many(organizations),
}));

export const organizationsRelations = relations(
  organizations,
  ({ one, many }) => ({
    enterprise: one(enterprises, {
      fields: [organizations.enterpriseId],
      references: [enterprises.id],
    }),
    members: many(organizationMembers),
    projects: many(projects),
  })
);

export const organizationMembersRelations = relations(
  organizationMembers,
  ({ one }) => ({
    organization: one(organizations, {
      fields: [organizationMembers.organizationId],
      references: [organizations.id],
    }),
  })
);

export const projectsRelations = relations(projects, ({ one }) => ({
  organization: one(organizations, {
    fields: [projects.organizationId],
    references: [organizations.id],
  }),
}));

// ============================================================================
// Type Exports
// ============================================================================

export type Enterprise = typeof enterprises.$inferSelect;
export type NewEnterprise = typeof enterprises.$inferInsert;

export type Organization = typeof organizations.$inferSelect;
export type NewOrganization = typeof organizations.$inferInsert;

export type OrganizationMember = typeof organizationMembers.$inferSelect;
export type NewOrganizationMember = typeof organizationMembers.$inferInsert;

export type Project = typeof projects.$inferSelect;
export type NewProject = typeof projects.$inferInsert;
