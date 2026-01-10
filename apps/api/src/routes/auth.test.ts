import { Hono } from "hono";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Env } from "../types/env";

// Mock the database client
const mockFindFirst = vi.fn();
const mockFindMany = vi.fn();
const mockUpdate = vi.fn();
const mockInsert = vi.fn();
const mockSet = vi.fn();
const mockWhere = vi.fn();
const mockReturning = vi.fn();
const mockValues = vi.fn();

const mockDb = {
  query: {
    organizationMembers: {
      findFirst: mockFindFirst,
      findMany: mockFindMany,
    },
    organizations: {
      findMany: mockFindMany,
    },
  },
  update: mockUpdate,
  insert: mockInsert,
};

const mockClient = {
  end: vi.fn(),
};

vi.mock("../db/client", () => ({
  createDb: vi.fn(() => Promise.resolve({ db: mockDb, client: mockClient })),
}));

// Mock fetch for WorkOS and GitHub API calls
const mockFetch = vi.fn();

// Mock environment
const MOCK_ENV = {
  WORKOS_API_KEY: "sk_test_workos_key",
  WORKOS_CLIENT_ID: "client_123",
  HYPERDRIVE: {
    connectionString: "postgres://test:test@localhost:5432/test",
  },
};

// Helper to make request with a fresh app instance
const makeRequest = async (
  method: "GET" | "POST",
  path: string,
  body?: unknown
): Promise<Response> => {
  // Import the routes fresh each time
  const authRoutes = (await import("./auth")).default;

  const app = new Hono<{ Bindings: Env }>();

  // Middleware to set auth context (simulating what authMiddleware does)
  app.use("*", async (c, next) => {
    c.set("auth" as never, { userId: "user-123" } as never);
    await next();
  });

  app.route("/auth", authRoutes);

  const options: RequestInit = {
    method,
    headers: {
      Authorization: "Bearer test-token",
      "Content-Type": "application/json",
    },
  };

  if (body) {
    options.body = JSON.stringify(body);
  }

  return app.request(path, options, MOCK_ENV);
};

// Factory for WorkOS user response
const createWorkOSUser = (
  overrides: Partial<{
    id: string;
    email: string;
    first_name: string;
    last_name: string;
    profile_picture_url: string;
  }> = {}
) => ({
  id: overrides.id ?? "user-123",
  email: overrides.email ?? "test@example.com",
  first_name: overrides.first_name ?? "Test",
  last_name: overrides.last_name ?? "User",
  profile_picture_url: overrides.profile_picture_url ?? null,
});

// Factory for WorkOS identities response
const createIdentitiesResponse = (
  identities: Array<{
    idp_id: string;
    type: string;
    provider: string;
  }> = []
) => ({
  data: identities,
});

describe("auth routes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockReset();

    // Setup mock chain for update
    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ returning: mockReturning });
    mockReturning.mockResolvedValue([]);

    // Setup mock chain for insert
    mockInsert.mockReturnValue({ values: mockValues });
    mockValues.mockResolvedValue([]);

    // Setup mock for findMany (organizations and organizationMembers)
    mockFindMany.mockResolvedValue([]);

    // Replace global fetch with mock
    global.fetch = mockFetch;
  });

  describe("POST /auth/sync-identity", () => {
    it("syncs identity with GitHub linked", async () => {
      // Mock WorkOS user fetch
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        // Mock WorkOS identities fetch
        .mockResolvedValueOnce({
          ok: true,
          json: () =>
            Promise.resolve(
              createIdentitiesResponse([
                { idp_id: "12345", type: "OAuth", provider: "GitHubOAuth" },
              ])
            ),
        })
        // Mock WorkOS Pipes token fetch
        .mockResolvedValueOnce({
          ok: true,
          json: () =>
            Promise.resolve({
              active: true,
              access_token: { token: "gho_test_token" },
            }),
        })
        // Mock GitHub user fetch (authenticated)
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({ login: "testuser" }),
        });

      mockReturning.mockResolvedValue([
        { organizationId: "organization-1", providerUsername: "testuser" },
        { organizationId: "organization-2", providerUsername: "testuser" },
      ]);

      const res = await makeRequest("POST", "/auth/sync-identity");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        user_id: "user-123",
        email: "test@example.com",
        first_name: "Test",
        last_name: "User",
        github_synced: true,
        github_user_id: "12345",
        github_username: "testuser",
        organizations_updated: 2,
        installer_orgs_linked: 0,
      });

      // Verify database was updated
      expect(mockUpdate).toHaveBeenCalled();
      expect(mockClient.end).toHaveBeenCalled();
    });

    it("returns user info when no GitHub identity linked", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createIdentitiesResponse([])),
        });

      const res = await makeRequest("POST", "/auth/sync-identity");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        user_id: "user-123",
        email: "test@example.com",
        first_name: "Test",
        last_name: "User",
        github_synced: false,
        github_username: null,
      });

      // Database should not be updated
      expect(mockUpdate).not.toHaveBeenCalled();
    });

    it("returns user info when identities fetch fails", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        .mockResolvedValueOnce({
          ok: false,
          text: () => Promise.resolve("Unauthorized"),
        });

      const res = await makeRequest("POST", "/auth/sync-identity");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        user_id: "user-123",
        email: "test@example.com",
        first_name: "Test",
        last_name: "User",
        github_synced: false,
        github_username: null,
      });
    });

    it("syncs identity even when WorkOS Pipes token fetch fails", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: () =>
            Promise.resolve(
              createIdentitiesResponse([
                { idp_id: "12345", type: "OAuth", provider: "GitHubOAuth" },
              ])
            ),
        })
        // WorkOS Pipes token fetch fails
        .mockResolvedValueOnce({
          ok: false,
          json: () => Promise.reject(new Error("Unauthorized")),
        });

      mockReturning.mockResolvedValue([
        { organizationId: "organization-1", providerUsername: null },
      ]);

      const res = await makeRequest("POST", "/auth/sync-identity");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toMatchObject({
        github_synced: true,
        github_user_id: "12345",
        github_username: null,
        organizations_updated: 1,
      });
    });

    it("syncs identity even when GitHub API fetch fails", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: () =>
            Promise.resolve(
              createIdentitiesResponse([
                { idp_id: "12345", type: "OAuth", provider: "GitHubOAuth" },
              ])
            ),
        })
        // WorkOS Pipes token fetch succeeds
        .mockResolvedValueOnce({
          ok: true,
          json: () =>
            Promise.resolve({
              active: true,
              access_token: { token: "gho_test_token" },
            }),
        })
        // GitHub API fetch fails
        .mockResolvedValueOnce({
          ok: false,
          json: () => Promise.reject(new Error("Not found")),
        });

      mockReturning.mockResolvedValue([
        { organizationId: "organization-1", providerUsername: null },
      ]);

      const res = await makeRequest("POST", "/auth/sync-identity");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toMatchObject({
        github_synced: true,
        github_user_id: "12345",
        github_username: null,
        organizations_updated: 1,
      });
    });

    it("returns 500 when WorkOS user fetch fails", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Internal Server Error"),
      });

      const res = await makeRequest("POST", "/auth/sync-identity");
      const json = await res.json();

      expect(res.status).toBe(500);
      expect(json).toEqual({ error: "Failed to fetch user details" });
    });

    it("filters out non-GitHub OAuth identities", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: () =>
            Promise.resolve(
              createIdentitiesResponse([
                {
                  idp_id: "google-123",
                  type: "OAuth",
                  provider: "GoogleOAuth",
                },
                {
                  idp_id: "microsoft-456",
                  type: "OAuth",
                  provider: "MicrosoftOAuth",
                },
              ])
            ),
        });

      const res = await makeRequest("POST", "/auth/sync-identity");
      const json = (await res.json()) as {
        github_synced: boolean;
        github_username: string | null;
      };

      expect(res.status).toBe(200);
      expect(json.github_synced).toBe(false);
      expect(json.github_username).toBeNull();
    });
  });

  describe("GET /auth/me", () => {
    it("returns user info with GitHub linked", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(createWorkOSUser()),
      });

      mockFindFirst.mockResolvedValue({
        id: "member-1",
        organizationId: "organization-1",
        userId: "user-123",
        providerUserId: "12345",
        providerUsername: "testuser",
      });

      const res = await makeRequest("GET", "/auth/me");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        user_id: "user-123",
        email: "test@example.com",
        first_name: "Test",
        last_name: "User",
        github_linked: true,
        github_user_id: "12345",
        github_username: "testuser",
      });

      expect(mockClient.end).toHaveBeenCalled();
    });

    it("returns user info without GitHub linked", async () => {
      // First call: get user details from WorkOS
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        // Second call: check WorkOS identities (since no GitHub linked in membership)
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createIdentitiesResponse([])),
        });

      mockFindFirst.mockResolvedValue({
        id: "member-1",
        organizationId: "organization-1",
        userId: "user-123",
        providerUserId: null,
        providerUsername: null,
      });

      const res = await makeRequest("GET", "/auth/me");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        user_id: "user-123",
        email: "test@example.com",
        first_name: "Test",
        last_name: "User",
        github_linked: false,
        github_user_id: null,
        github_username: null,
      });
    });

    it("returns user info when not member of any organization", async () => {
      // First call: get user details from WorkOS
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createWorkOSUser()),
        })
        // Second call: check WorkOS identities (since no membership)
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createIdentitiesResponse([])),
        });

      mockFindFirst.mockResolvedValue(undefined);

      const res = await makeRequest("GET", "/auth/me");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        user_id: "user-123",
        email: "test@example.com",
        first_name: "Test",
        last_name: "User",
        github_linked: false,
        github_user_id: null,
        github_username: null,
      });
    });

    it("returns 500 when WorkOS user fetch fails", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Server Error"),
      });

      const res = await makeRequest("GET", "/auth/me");
      const json = await res.json();

      expect(res.status).toBe(500);
      expect(json).toEqual({ error: "Failed to fetch user details" });
    });

    it("handles user without first/last name", async () => {
      // First call: get user details from WorkOS (no first/last name)
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () =>
            Promise.resolve({
              id: "user-123",
              email: "test@example.com",
            }),
        })
        // Second call: check WorkOS identities (since no membership)
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(createIdentitiesResponse([])),
        });

      mockFindFirst.mockResolvedValue(undefined);

      const res = await makeRequest("GET", "/auth/me");
      const json = (await res.json()) as {
        first_name?: string;
        last_name?: string;
      };

      expect(res.status).toBe(200);
      expect(json.first_name).toBeUndefined();
      expect(json.last_name).toBeUndefined();
    });
  });
});
