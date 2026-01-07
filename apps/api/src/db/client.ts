import { drizzle } from "drizzle-orm/node-postgres";
import { Client } from "pg";
import type { Env } from "../types/env";
// biome-ignore lint/performance/noNamespaceImport: Drizzle requires namespace import for relational queries
import * as schema from "./schema";

export const createDb = async (env: Env) => {
  const client = new Client({
    connectionString: env.HYPERDRIVE.connectionString,
  });
  await client.connect();
  const db = drizzle({ client, schema });
  return { db, client };
};

export type Database = Awaited<ReturnType<typeof createDb>>["db"];
