ALTER TABLE "projects" ADD COLUMN "handle" varchar(255) NOT NULL;--> statement-breakpoint
CREATE UNIQUE INDEX "projects_org_handle_idx" ON "projects" USING btree ("organization_id","handle");