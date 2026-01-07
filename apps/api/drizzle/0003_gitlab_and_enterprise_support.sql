-- Multi-provider support (GitLab) and enterprise stub
-- This migration:
-- 1. Creates enterprises table for future org grouping
-- 2. Adds GitLab-specific columns to organizations
-- 3. Makes provider_installation_id nullable (GitLab has no installations)
-- 4. Adds composite unique constraint on (provider, provider_account_id)

-- Step 1: Create enterprises table
CREATE TABLE "enterprises" (
  "id" varchar(36) PRIMARY KEY NOT NULL,
  "name" varchar(255) NOT NULL,
  "slug" varchar(255) NOT NULL UNIQUE,
  "suspended_at" timestamp,
  "deleted_at" timestamp,
  "created_at" timestamp DEFAULT now() NOT NULL,
  "updated_at" timestamp DEFAULT now() NOT NULL
);--> statement-breakpoint

CREATE UNIQUE INDEX "enterprises_slug_idx" ON "enterprises" USING btree ("slug");--> statement-breakpoint

-- Step 2: Add enterprise FK to organizations
ALTER TABLE "organizations" ADD COLUMN "enterprise_id" varchar(36) REFERENCES "enterprises"("id") ON DELETE SET NULL;--> statement-breakpoint

CREATE INDEX "organizations_enterprise_id_idx" ON "organizations" USING btree ("enterprise_id");--> statement-breakpoint

-- Step 3: Add GitLab-specific columns to organizations
ALTER TABLE "organizations" ADD COLUMN "provider_access_token_encrypted" varchar(500);--> statement-breakpoint
ALTER TABLE "organizations" ADD COLUMN "provider_access_token_expires_at" timestamp;--> statement-breakpoint
ALTER TABLE "organizations" ADD COLUMN "provider_webhook_secret" varchar(255);--> statement-breakpoint

-- Step 4: Make provider_installation_id nullable for GitLab support
-- First drop the existing unique index and constraint
DROP INDEX "organizations_provider_installation_id_idx";--> statement-breakpoint
ALTER TABLE "organizations" DROP CONSTRAINT "organizations_provider_installation_id_unique";--> statement-breakpoint

-- Make the column nullable
ALTER TABLE "organizations" ALTER COLUMN "provider_installation_id" DROP NOT NULL;--> statement-breakpoint

-- Recreate as partial unique index (only enforces uniqueness when value is not null)
CREATE UNIQUE INDEX "organizations_provider_installation_id_idx" ON "organizations" USING btree ("provider_installation_id") WHERE "provider_installation_id" IS NOT NULL;--> statement-breakpoint

-- Step 5: Add composite unique constraint for provider + account ID
-- This ensures the same account can't be registered twice for the same provider
CREATE UNIQUE INDEX "organizations_provider_account_unique_idx" ON "organizations" USING btree ("provider", "provider_account_id");
