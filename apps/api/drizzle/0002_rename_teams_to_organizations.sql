-- Rename teams to organizations
-- This migration renames all team-related tables, columns, and constraints to organization

-- Step 1: Drop foreign key constraints (must be done before renaming)
ALTER TABLE "projects" DROP CONSTRAINT "projects_team_id_teams_id_fk";--> statement-breakpoint
ALTER TABLE "team_members" DROP CONSTRAINT "team_members_team_id_teams_id_fk";--> statement-breakpoint

-- Step 2: Drop indexes that reference old names
DROP INDEX "projects_team_id_idx";--> statement-breakpoint
DROP INDEX "projects_team_repo_idx";--> statement-breakpoint
DROP INDEX "team_members_team_user_idx";--> statement-breakpoint
DROP INDEX "team_members_user_id_idx";--> statement-breakpoint
DROP INDEX "teams_slug_idx";--> statement-breakpoint
DROP INDEX "teams_provider_installation_id_idx";--> statement-breakpoint
DROP INDEX "teams_provider_account_idx";--> statement-breakpoint

-- Step 3: Rename tables
ALTER TABLE "teams" RENAME TO "organizations";--> statement-breakpoint
ALTER TABLE "team_members" RENAME TO "organization_members";--> statement-breakpoint

-- Step 4: Rename columns
ALTER TABLE "projects" RENAME COLUMN "team_id" TO "organization_id";--> statement-breakpoint
ALTER TABLE "organization_members" RENAME COLUMN "team_id" TO "organization_id";--> statement-breakpoint

-- Step 5: Rename enum (team_role -> organization_role)
ALTER TYPE "team_role" RENAME TO "organization_role";--> statement-breakpoint

-- Step 6: Rename constraints on organizations table
ALTER TABLE "organizations" RENAME CONSTRAINT "teams_slug_unique" TO "organizations_slug_unique";--> statement-breakpoint
ALTER TABLE "organizations" RENAME CONSTRAINT "teams_provider_installation_id_unique" TO "organizations_provider_installation_id_unique";--> statement-breakpoint

-- Step 7: Recreate foreign key constraints with new names
ALTER TABLE "projects" ADD CONSTRAINT "projects_organization_id_organizations_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organizations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "organization_members" ADD CONSTRAINT "organization_members_organization_id_organizations_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organizations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint

-- Step 8: Recreate indexes with new names
CREATE INDEX "projects_organization_id_idx" ON "projects" USING btree ("organization_id");--> statement-breakpoint
CREATE UNIQUE INDEX "projects_organization_repo_idx" ON "projects" USING btree ("organization_id","provider_repo_id");--> statement-breakpoint
CREATE UNIQUE INDEX "organization_members_organization_user_idx" ON "organization_members" USING btree ("organization_id","user_id");--> statement-breakpoint
CREATE INDEX "organization_members_user_id_idx" ON "organization_members" USING btree ("user_id");--> statement-breakpoint
CREATE UNIQUE INDEX "organizations_slug_idx" ON "organizations" USING btree ("slug");--> statement-breakpoint
CREATE UNIQUE INDEX "organizations_provider_installation_id_idx" ON "organizations" USING btree ("provider_installation_id");--> statement-breakpoint
CREATE INDEX "organizations_provider_account_idx" ON "organizations" USING btree ("provider","provider_account_id");
