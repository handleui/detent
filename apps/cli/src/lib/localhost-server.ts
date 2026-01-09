import { randomBytes } from "node:crypto";
import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

interface CallbackResult {
  code: string;
  state: string;
}

interface CallbackServer {
  port: number;
  waitForCallback: () => Promise<CallbackResult>;
  close: () => void;
}

/**
 * Start a temporary localhost server to receive OAuth callback
 */
export const startCallbackServer = (
  expectedState: string
): Promise<CallbackServer> => {
  return new Promise((resolve, reject) => {
    let callbackPromiseResolve: (result: CallbackResult) => void;
    let callbackPromiseReject: (error: Error) => void;

    const callbackPromise = new Promise<CallbackResult>((res, rej) => {
      callbackPromiseResolve = res;
      callbackPromiseReject = rej;
    });

    const server = createServer((req: IncomingMessage, res: ServerResponse) => {
      const url = new URL(req.url ?? "/", "http://localhost");

      if (url.pathname === "/callback") {
        const code = url.searchParams.get("code");
        const state = url.searchParams.get("state");

        // Send success page to browser
        res.writeHead(200, { "Content-Type": "text/html" });
        res.end(`
          <!DOCTYPE html>
          <html>
            <head><title>Detent CLI</title></head>
            <body style="font-family: system-ui; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0;">
              <div style="text-align: center;">
                <h1>Authentication successful!</h1>
                <p>You can close this window and return to the terminal.</p>
              </div>
            </body>
          </html>
        `);

        // Verify state
        if (state !== expectedState) {
          callbackPromiseReject(
            new Error("State mismatch - possible CSRF attack")
          );
          return;
        }

        if (!code) {
          callbackPromiseReject(new Error("No authorization code received"));
          return;
        }

        callbackPromiseResolve({ code, state });
      } else {
        res.writeHead(404);
        res.end("Not found");
      }
    });

    // Use port 0 to get a random available port
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("Failed to get server address"));
        return;
      }

      const port = address.port;

      // Set timeout (5 minutes)
      const timeout = setTimeout(
        () => {
          callbackPromiseReject(
            new Error("Authentication timed out. Please try again.")
          );
          server.close();
        },
        5 * 60 * 1000
      );

      resolve({
        port,
        waitForCallback: async () => {
          try {
            const result = await callbackPromise;
            clearTimeout(timeout);
            return result;
          } finally {
            server.close();
          }
        },
        close: () => {
          clearTimeout(timeout);
          server.close();
        },
      });
    });

    server.on("error", reject);
  });
};

/**
 * Generate a cryptographically secure state string
 */
export const generateState = (): string => randomBytes(32).toString("hex");
