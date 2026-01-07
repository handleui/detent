ALTER TABLE "organizations" ADD COLUMN "last_synced_at" timestamp;--> statement-breakpoint
CREATE INDEX "projects_provider_repo_id_idx" ON "projects" USING btree ("provider_repo_id");