import { beforeEach, describe, expect, it, vi } from "vitest";
import app from "./webhooks";

// Mock the webhook signature middleware to bypass signature verification in tests
vi.mock("../middleware/webhook-signature", () => ({
  webhookSignatureMiddleware: vi.fn(async (c, next) => {
    const rawBody = await c.req.text();
    c.set("webhookPayload", JSON.parse(rawBody));
    await next();
  }),
}));

// Mock the database client
const mockSelect = vi.fn();
const mockFrom = vi.fn();
const mockWhere = vi.fn();
const mockLimit = vi.fn();
const mockInsert = vi.fn();
const mockValues = vi.fn();
const mockUpdate = vi.fn();
const mockSet = vi.fn();

const mockDb = {
  select: mockSelect,
  insert: mockInsert,
  update: mockUpdate,
};

const mockClient = {
  end: vi.fn(),
};

vi.mock("../db/client", () => ({
  createDb: vi.fn(() => Promise.resolve({ db: mockDb, client: mockClient })),
}));

// Mock crypto.randomUUID for deterministic organization IDs
const mockUUID = "test-uuid-1234-5678-9abc-def012345678";
vi.spyOn(crypto, "randomUUID").mockImplementation(() => mockUUID);

// Mock environment
const MOCK_ENV = {
  GITHUB_WEBHOOK_SECRET: "test-webhook-secret",
  GITHUB_APP_ID: "123456",
  GITHUB_CLIENT_ID: "test-client-id",
  GITHUB_APP_PRIVATE_KEY: "test-private-key",
  HYPERDRIVE: {
    connectionString: "postgres://test:test@localhost:5432/test",
  },
  WORKOS_CLIENT_ID: "test-workos-client",
  WORKOS_API_KEY: "test-workos-key",
};

// Factory for installation payloads
const createInstallationPayload = (
  action: "created" | "deleted" | "suspend" | "unsuspend",
  overrides: Partial<{
    installationId: number;
    accountId: number;
    accountLogin: string;
    accountType: "Organization" | "User";
    avatarUrl: string;
  }> = {}
) => ({
  action,
  installation: {
    id: overrides.installationId ?? 12_345_678,
    account: {
      id: overrides.accountId ?? 98_765_432,
      login: overrides.accountLogin ?? "test-org",
      type: overrides.accountType ?? ("Organization" as const),
      avatar_url: overrides.avatarUrl ?? "https://avatars.example.com/u/123",
    },
  },
});

// Helper to make webhook request
const makeWebhookRequest = async (
  event: string,
  payload: unknown
): Promise<Response> => {
  const body = JSON.stringify(payload);

  const response = await app.request(
    "/github",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-GitHub-Event": event,
        "X-GitHub-Delivery": "test-delivery-id",
        "X-Hub-Signature-256": "sha256=mocked",
      },
      body,
    },
    MOCK_ENV
  );

  return response;
};

// Response JSON type for installation events
interface InstallationResponse {
  message: string;
  organization_id?: string;
  organization_slug?: string;
  account?: string;
  action?: string;
  error?: string;
}

describe("webhooks - installation events", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Setup mock chain for select queries
    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });
    mockLimit.mockResolvedValue([]);

    // Setup mock chain for insert
    mockInsert.mockReturnValue({ values: mockValues });
    mockValues.mockResolvedValue(undefined);

    // Setup mock chain for update
    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: mockWhere });
  });

  describe("installation.created", () => {
    it("creates a new organization with correct fields", async () => {
      const payload = createInstallationPayload("created");

      const res = await makeWebhookRequest("installation", payload);
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        message: "installation created",
        organization_id: mockUUID,
        organization_slug: "test-org",
        account: "test-org",
        projects_created: 0,
      });

      // Verify insert was called with correct values
      expect(mockInsert).toHaveBeenCalled();
      expect(mockValues).toHaveBeenCalledWith({
        id: mockUUID,
        name: "test-org",
        slug: "test-org",
        provider: "github",
        providerAccountId: "98765432",
        providerAccountLogin: "test-org",
        providerAccountType: "organization",
        providerInstallationId: "12345678",
        providerAvatarUrl: "https://avatars.example.com/u/123",
      });
    });

    it("creates organization for User account type", async () => {
      const payload = createInstallationPayload("created", {
        accountType: "User",
        accountLogin: "my-user",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      expect(json.organization_slug).toBe("my-user");

      expect(mockValues).toHaveBeenCalledWith(
        expect.objectContaining({
          providerAccountType: "user",
        })
      );
    });

    it("normalizes slug to lowercase with hyphens", async () => {
      const payload = createInstallationPayload("created", {
        accountLogin: "My_Test_Org",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      expect(json.organization_slug).toBe("my-test-org");
    });

    it("handles null avatar URL", async () => {
      const payload = createInstallationPayload("created");
      // biome-ignore lint/performance/noDelete: Testing undefined field behavior
      delete (payload.installation.account as Record<string, unknown>)
        .avatar_url;

      const res = await makeWebhookRequest("installation", payload);

      expect(res.status).toBe(200);
      expect(mockValues).toHaveBeenCalledWith(
        expect.objectContaining({
          providerAvatarUrl: null,
        })
      );
    });
  });

  describe("idempotency - duplicate installation", () => {
    it("returns success when organization already exists for installation", async () => {
      // Mock existing organization found
      mockLimit.mockResolvedValueOnce([
        {
          id: "existing-organization-id",
          slug: "existing-organization",
        },
      ]);

      const payload = createInstallationPayload("created");

      const res = await makeWebhookRequest("installation", payload);
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        message: "installation already exists",
        organization_id: "existing-organization-id",
        organization_slug: "existing-organization",
        account: "test-org",
      });

      // Verify no insert was attempted
      expect(mockInsert).not.toHaveBeenCalled();
    });
  });

  describe("slug collision handling", () => {
    it("appends suffix when slug already exists", async () => {
      // First query: no existing organization with this installation
      mockLimit.mockResolvedValueOnce([]);
      // Second query: slug conflict found
      mockLimit.mockResolvedValueOnce([{ id: "other-organization-id" }]);
      // Third query: suffixed slug is available
      mockLimit.mockResolvedValueOnce([]);

      const payload = createInstallationPayload("created", {
        accountLogin: "test-org",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      expect(json.organization_slug).toBe("test-org-1");
    });

    it("increments suffix for multiple collisions", async () => {
      // First query: no existing organization with this installation
      mockLimit.mockResolvedValueOnce([]);
      // Queries 2-4: slug conflicts
      mockLimit.mockResolvedValueOnce([{ id: "other-organization-1" }]);
      mockLimit.mockResolvedValueOnce([{ id: "other-organization-2" }]);
      mockLimit.mockResolvedValueOnce([{ id: "other-organization-3" }]);
      // Query 5: suffixed slug is available
      mockLimit.mockResolvedValueOnce([]);

      const payload = createInstallationPayload("created", {
        accountLogin: "popular-name",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      expect(json.organization_slug).toBe("popular-name-3");
    });

    it("falls back to UUID suffix after max attempts", async () => {
      // First query: no existing organization with this installation
      mockLimit.mockResolvedValueOnce([]);
      // Next 10 queries: all slug conflicts
      for (let i = 0; i < 10; i++) {
        mockLimit.mockResolvedValueOnce([{ id: `organization-${i}` }]);
      }

      const payload = createInstallationPayload("created", {
        accountLogin: "super-popular",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      // Falls back to UUID prefix (first 8 chars of mockUUID)
      expect(json.organization_slug).toBe("super-popular-test-uui");
    });
  });

  describe("installation.deleted", () => {
    it("soft-deletes the organization by setting deletedAt", async () => {
      const payload = createInstallationPayload("deleted");

      const res = await makeWebhookRequest("installation", payload);
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        message: "installation deleted",
        account: "test-org",
      });

      // Verify update was called with correct fields
      expect(mockUpdate).toHaveBeenCalled();
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          deletedAt: expect.any(Date),
          updatedAt: expect.any(Date),
        })
      );
    });
  });

  describe("installation.suspend", () => {
    it("marks organization as suspended by setting suspendedAt", async () => {
      const payload = createInstallationPayload("suspend");

      const res = await makeWebhookRequest("installation", payload);
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        message: "installation suspended",
        account: "test-org",
      });

      expect(mockUpdate).toHaveBeenCalled();
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          suspendedAt: expect.any(Date),
          updatedAt: expect.any(Date),
        })
      );
    });
  });

  describe("installation.unsuspend", () => {
    it("clears suspension by setting suspendedAt to null", async () => {
      const payload = {
        action: "unsuspend",
        installation: {
          id: 12_345_678,
          account: {
            id: 98_765_432,
            login: "test-org",
            type: "Organization" as const,
          },
        },
      };

      const res = await makeWebhookRequest("installation", payload);
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        message: "installation unsuspended",
        account: "test-org",
      });

      expect(mockUpdate).toHaveBeenCalled();
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          suspendedAt: null,
          updatedAt: expect.any(Date),
        })
      );
    });
  });

  describe("error handling", () => {
    it("returns 500 on database error", async () => {
      mockLimit.mockRejectedValueOnce(new Error("Database connection failed"));

      const payload = createInstallationPayload("created");

      const res = await makeWebhookRequest("installation", payload);
      const json = await res.json();

      expect(res.status).toBe(500);
      expect(json).toEqual({
        message: "installation error",
        error: "Database connection failed",
      });
    });

    it("closes database connection on success", async () => {
      const payload = createInstallationPayload("deleted");

      await makeWebhookRequest("installation", payload);

      expect(mockClient.end).toHaveBeenCalled();
    });

    it("closes database connection on error", async () => {
      mockLimit.mockRejectedValueOnce(new Error("Test error"));

      const payload = createInstallationPayload("created");

      await makeWebhookRequest("installation", payload);

      expect(mockClient.end).toHaveBeenCalled();
    });
  });

  describe("unknown installation actions", () => {
    it("ignores new_permissions_accepted action", async () => {
      const payload = {
        action: "new_permissions_accepted",
        installation: {
          id: 12_345_678,
          account: {
            id: 98_765_432,
            login: "test-org",
            type: "Organization" as const,
          },
        },
      };

      const res = await makeWebhookRequest("installation", payload);
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json).toEqual({
        message: "ignored",
        action: "new_permissions_accepted",
      });

      expect(mockInsert).not.toHaveBeenCalled();
      expect(mockUpdate).not.toHaveBeenCalled();
    });
  });
});

describe("webhooks - ping event", () => {
  it("responds to ping with pong and zen", async () => {
    const payload = {
      zen: "Speak like a human.",
      hook_id: 123_456,
    };

    const res = await makeWebhookRequest("ping", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "pong",
      zen: "Speak like a human.",
    });
  });
});
