ALTER TABLE "team_members" ADD COLUMN "provider_user_id" varchar(255);--> statement-breakpoint
ALTER TABLE "team_members" ADD COLUMN "provider_username" varchar(255);--> statement-breakpoint
ALTER TABLE "team_members" ADD COLUMN "provider_linked_at" timestamp;
