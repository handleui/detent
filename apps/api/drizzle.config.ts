import { config } from "dotenv";
import { defineConfig } from "drizzle-kit";

config({ path: ".dev.vars" });

const {
  PLANETSCALE_HOST,
  PLANETSCALE_USERNAME,
  PLANETSCALE_PASSWORD,
  PLANETSCALE_DBNAME = "postgres",
} = process.env;

const url = `postgresql://${PLANETSCALE_USERNAME}:${PLANETSCALE_PASSWORD}@${PLANETSCALE_HOST}:5432/${PLANETSCALE_DBNAME}?sslmode=require`;

export default defineConfig({
  schema: "./src/db/schema.ts",
  out: "./drizzle",
  dialect: "postgresql",
  dbCredentials: { url },
});
