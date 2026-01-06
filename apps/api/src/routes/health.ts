import { Hono } from "hono";

const app = new Hono();

app.get("/", (c) =>
  c.json({
    status: "ok",
    version: "0.1.0",
  })
);

export default app;
