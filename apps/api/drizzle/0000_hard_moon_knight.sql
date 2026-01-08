CREATE TYPE "public"."account_type" AS ENUM('organization', 'user');--> statement-breakpoint
CREATE TYPE "public"."organization_role" AS ENUM('owner', 'admin', 'member');--> statement-breakpoint
CREATE TYPE "public"."provider" AS ENUM('github', 'gitlab');--> statement-breakpoint
CREATE TABLE "enterprises" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"name" varchar(255) NOT NULL,
	"slug" varchar(255) NOT NULL,
	"suspended_at" timestamp,
	"deleted_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "enterprises_slug_unique" UNIQUE("slug")
);
--> statement-breakpoint
CREATE TABLE "organization_members" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"organization_id" varchar(36) NOT NULL,
	"user_id" varchar(255) NOT NULL,
	"role" "organization_role" DEFAULT 'member' NOT NULL,
	"provider_user_id" varchar(255),
	"provider_username" varchar(255),
	"provider_linked_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "organizations" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"name" varchar(255) NOT NULL,
	"slug" varchar(255) NOT NULL,
	"enterprise_id" varchar(36),
	"provider" "provider" NOT NULL,
	"provider_account_id" varchar(255) NOT NULL,
	"provider_account_login" varchar(255) NOT NULL,
	"provider_account_type" "account_type" NOT NULL,
	"provider_avatar_url" varchar(500),
	"provider_installation_id" varchar(255),
	"provider_access_token_encrypted" varchar(500),
	"provider_access_token_expires_at" timestamp,
	"provider_webhook_secret" varchar(255),
	"suspended_at" timestamp,
	"deleted_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "organizations_slug_unique" UNIQUE("slug")
);
--> statement-breakpoint
CREATE TABLE "projects" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"organization_id" varchar(36) NOT NULL,
	"provider_repo_id" varchar(255) NOT NULL,
	"provider_repo_name" varchar(255) NOT NULL,
	"provider_repo_full_name" varchar(500) NOT NULL,
	"provider_default_branch" varchar(255),
	"is_private" boolean DEFAULT false NOT NULL,
	"removed_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "organization_members" ADD CONSTRAINT "organization_members_organization_id_organizations_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organizations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "organizations" ADD CONSTRAINT "organizations_enterprise_id_enterprises_id_fk" FOREIGN KEY ("enterprise_id") REFERENCES "public"."enterprises"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_organization_id_organizations_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organizations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "enterprises_slug_idx" ON "enterprises" USING btree ("slug");--> statement-breakpoint
CREATE UNIQUE INDEX "organization_members_org_user_idx" ON "organization_members" USING btree ("organization_id","user_id");--> statement-breakpoint
CREATE INDEX "organization_members_user_id_idx" ON "organization_members" USING btree ("user_id");--> statement-breakpoint
CREATE UNIQUE INDEX "organizations_slug_idx" ON "organizations" USING btree ("slug");--> statement-breakpoint
CREATE UNIQUE INDEX "organizations_provider_installation_id_idx" ON "organizations" USING btree ("provider_installation_id");--> statement-breakpoint
CREATE UNIQUE INDEX "organizations_provider_account_unique_idx" ON "organizations" USING btree ("provider","provider_account_id");--> statement-breakpoint
CREATE INDEX "organizations_enterprise_id_idx" ON "organizations" USING btree ("enterprise_id");--> statement-breakpoint
CREATE INDEX "projects_organization_id_idx" ON "projects" USING btree ("organization_id");--> statement-breakpoint
CREATE UNIQUE INDEX "projects_org_repo_idx" ON "projects" USING btree ("organization_id","provider_repo_id");--> statement-breakpoint
CREATE INDEX "projects_provider_repo_full_name_idx" ON "projects" USING btree ("provider_repo_full_name");