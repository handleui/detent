import { config } from "dotenv";
import { defineConfig } from "drizzle-kit";

config({ path: ".dev.vars" });

const required = ["DB_HOST", "DB_USERNAME", "DB_PASSWORD"] as const;
const missing = required.filter((key) => !process.env[key]);
if (missing.length > 0) {
  throw new Error(`Missing required env vars: ${missing.join(", ")}`);
}

const { DB_HOST, DB_USERNAME, DB_PASSWORD } = process.env as Record<
  (typeof required)[number],
  string
>;

const url = `postgresql://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:5432/postgres?sslmode=require`;

export default defineConfig({
  schema: "./src/db/schema.ts",
  out: "./drizzle",
  dialect: "postgresql",
  dbCredentials: { url },
});
