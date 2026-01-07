import { config } from "dotenv";
import { defineConfig } from "drizzle-kit";

config({ path: ".dev.vars" });

const { DB_HOST, DB_USERNAME, DB_PASSWORD } = process.env;

const url = `postgresql://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:5432/postgres?sslmode=require`;

export default defineConfig({
  schema: "./src/db/schema.ts",
  out: "./drizzle",
  dialect: "postgresql",
  dbCredentials: { url },
});
