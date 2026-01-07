// biome-ignore-all lint/performance/noBarrelFile: intentional barrel export for db module
export { createDb, type Database } from "./client";
export * from "./schema";
