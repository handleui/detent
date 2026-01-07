import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import type { Env } from "../types/env";
// biome-ignore lint/performance/noNamespaceImport: Drizzle requires namespace import for relational queries
import * as schema from "./schema";

export const createDb = (env: Env) => {
  const client = postgres(env.HYPERDRIVE.connectionString, {
    prepare: false,
    max: 5,
    fetch_types: false,
  });
  return drizzle({ client, schema });
};

export type Database = ReturnType<typeof createDb>;
