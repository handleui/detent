import type { MetadataRoute } from "next";

const manifest = (): MetadataRoute.Manifest => ({
  name: "Detent Navigator",
  short_name: "Navigator",
  description:
    "Authenticate and manage your Detent CLI sessions. Securely connect your development environment to Detent services.",
  start_url: "/",
  display: "standalone",
  background_color: "#000000",
  theme_color: "#000000",
});

export default manifest;
