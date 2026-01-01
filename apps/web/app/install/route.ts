import { generateInstallScript } from "../../lib/install-script";

export const GET = () =>
  new Response(generateInstallScript(), {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
    },
  });
