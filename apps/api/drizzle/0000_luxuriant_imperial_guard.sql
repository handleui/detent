CREATE TYPE "public"."account_type" AS ENUM('organization', 'user');--> statement-breakpoint
CREATE TYPE "public"."provider" AS ENUM('github', 'gitlab');--> statement-breakpoint
CREATE TYPE "public"."team_role" AS ENUM('owner', 'admin', 'member');--> statement-breakpoint
CREATE TABLE "projects" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"team_id" varchar(36) NOT NULL,
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
CREATE TABLE "team_members" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"team_id" varchar(36) NOT NULL,
	"user_id" varchar(255) NOT NULL,
	"role" "team_role" DEFAULT 'member' NOT NULL,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "teams" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"name" varchar(255) NOT NULL,
	"slug" varchar(255) NOT NULL,
	"provider" "provider" NOT NULL,
	"provider_account_id" varchar(255) NOT NULL,
	"provider_account_login" varchar(255) NOT NULL,
	"provider_account_type" "account_type" NOT NULL,
	"provider_installation_id" varchar(255) NOT NULL,
	"provider_avatar_url" varchar(500),
	"suspended_at" timestamp,
	"deleted_at" timestamp,
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "teams_slug_unique" UNIQUE("slug"),
	CONSTRAINT "teams_provider_installation_id_unique" UNIQUE("provider_installation_id")
);
--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_team_id_teams_id_fk" FOREIGN KEY ("team_id") REFERENCES "public"."teams"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "team_members" ADD CONSTRAINT "team_members_team_id_teams_id_fk" FOREIGN KEY ("team_id") REFERENCES "public"."teams"("id") ON DELETE no action ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "projects_team_id_idx" ON "projects" USING btree ("team_id");--> statement-breakpoint
CREATE UNIQUE INDEX "projects_team_repo_idx" ON "projects" USING btree ("team_id","provider_repo_id");--> statement-breakpoint
CREATE INDEX "projects_provider_repo_full_name_idx" ON "projects" USING btree ("provider_repo_full_name");--> statement-breakpoint
CREATE UNIQUE INDEX "team_members_team_user_idx" ON "team_members" USING btree ("team_id","user_id");--> statement-breakpoint
CREATE INDEX "team_members_user_id_idx" ON "team_members" USING btree ("user_id");--> statement-breakpoint
CREATE UNIQUE INDEX "teams_slug_idx" ON "teams" USING btree ("slug");--> statement-breakpoint
CREATE UNIQUE INDEX "teams_provider_installation_id_idx" ON "teams" USING btree ("provider_installation_id");--> statement-breakpoint
CREATE INDEX "teams_provider_account_idx" ON "teams" USING btree ("provider","provider_account_id");