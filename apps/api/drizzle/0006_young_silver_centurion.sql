CREATE TYPE "public"."invitation_status" AS ENUM('pending', 'accepted', 'expired', 'revoked');--> statement-breakpoint
CREATE TABLE "invitations" (
	"id" varchar(36) PRIMARY KEY NOT NULL,
	"organization_id" varchar(36) NOT NULL,
	"email" varchar(255) NOT NULL,
	"role" "organization_role" DEFAULT 'member' NOT NULL,
	"token" varchar(64) NOT NULL,
	"status" "invitation_status" DEFAULT 'pending' NOT NULL,
	"expires_at" timestamp NOT NULL,
	"invited_by" varchar(255) NOT NULL,
	"accepted_at" timestamp,
	"accepted_by_user_id" varchar(255),
	"revoked_at" timestamp,
	"revoked_by" varchar(255),
	"created_at" timestamp DEFAULT now() NOT NULL,
	"updated_at" timestamp DEFAULT now() NOT NULL,
	CONSTRAINT "invitations_token_unique" UNIQUE("token")
);
--> statement-breakpoint
ALTER TABLE "invitations" ADD CONSTRAINT "invitations_organization_id_organizations_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organizations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "invitations_token_idx" ON "invitations" USING btree ("token");--> statement-breakpoint
CREATE INDEX "invitations_org_status_idx" ON "invitations" USING btree ("organization_id","status");--> statement-breakpoint
CREATE INDEX "invitations_email_idx" ON "invitations" USING btree ("email");