import { randomBytes } from "node:crypto";
import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import type { Socket } from "node:net";

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
 * Optimized for fast shutdown by tracking and destroying connections
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

    // Track connections for fast shutdown
    const connections = new Set<Socket>();

    const server = createServer((req: IncomingMessage, res: ServerResponse) => {
      const url = new URL(req.url ?? "/", "http://localhost");

      if (url.pathname === "/callback") {
        const code = url.searchParams.get("code");
        const state = url.searchParams.get("state");

        // Minimal response - Navigator already showed success page
        // Try to close the browser tab automatically
        res.writeHead(200, { "Content-Type": "text/html" });
        res.end(
          "<!DOCTYPE html><html><head><script>window.close()</script></head><body></body></html>"
        );

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
        res.end();
      }
    });

    // Track connections for fast shutdown
    server.on("connection", (socket: Socket) => {
      connections.add(socket);
      socket.on("close", () => connections.delete(socket));
    });

    // Force-close all connections and shutdown server immediately
    const forceClose = () => {
      for (const socket of connections) {
        socket.destroy();
      }
      connections.clear();
      server.close();
    };

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
          forceClose();
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
            forceClose();
          }
        },
        close: () => {
          clearTimeout(timeout);
          forceClose();
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
