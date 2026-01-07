import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

// Mock the webhook signature middleware to bypass signature verification in tests
// vi.mock must be defined before dynamic import
vi.mock("../middleware/webhook-signature", () => ({
  webhookSignatureMiddleware: vi.fn(
    async (
      c: {
        req: { text: () => Promise<string> };
        set: (key: string, value: unknown) => void;
      },
      next: () => Promise<void>
    ) => {
      const rawBody = await c.req.text();
      c.set("webhookPayload", JSON.parse(rawBody));
      await next();
    }
  ),
}));

// Dynamic import required for bun test to properly apply mocks
let app: Awaited<typeof import("./webhooks")>["default"];

beforeAll(async () => {
  const module = await import("./webhooks");
  app = module.default;
});

// Mock the database client - defined before vi.mock to ensure proper closure capture
const mockSelect = vi.fn();
const mockFrom = vi.fn();
const mockWhere = vi.fn();
const mockLimit = vi.fn();
const mockInsert = vi.fn();
const mockValues = vi.fn();
const mockOnConflictDoNothing = vi.fn();
const mockUpdate = vi.fn();
const mockSet = vi.fn();
const mockClientEnd = vi.fn();

vi.mock("../db/client", () => ({
  createDb: vi.fn(() =>
    Promise.resolve({
      db: {
        select: mockSelect,
        insert: mockInsert,
        update: mockUpdate,
      },
      client: {
        end: mockClientEnd,
      },
    })
  ),
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
    // Queries can either:
    // 1. Chain with .limit() for single record lookup (installation check)
    // 2. Be awaited directly for array results (slug lookup with inArray)
    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });

    // Create a thenable object that also has a limit method
    // This handles both query patterns used in the code
    const createQueryResult = (result: unknown[] = []) => {
      const queryResult = Promise.resolve(result) as Promise<unknown[]> & {
        limit: ReturnType<typeof vi.fn>;
      };
      queryResult.limit = mockLimit;
      return queryResult;
    };

    mockWhere.mockImplementation(() => createQueryResult([]));
    mockLimit.mockResolvedValue([]);

    // Setup mock chain for insert
    mockInsert.mockReturnValue({ values: mockValues });
    mockValues.mockReturnValue({
      onConflictDoNothing: mockOnConflictDoNothing,
    });
    mockOnConflictDoNothing.mockResolvedValue(undefined);

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
        organization_slug: "gh/test-org",
        account: "test-org",
        projects_created: 0,
      });

      // Verify insert was called with correct values
      expect(mockInsert).toHaveBeenCalled();
      expect(mockValues).toHaveBeenCalledWith({
        id: mockUUID,
        name: "test-org",
        slug: "gh/test-org",
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
      expect(json.organization_slug).toBe("gh/my-user");

      expect(mockValues).toHaveBeenCalledWith(
        expect.objectContaining({
          providerAccountType: "user",
        })
      );
    });

    it("normalizes slug to lowercase with gh/ prefix", async () => {
      const payload = createInstallationPayload("created", {
        accountLogin: "My_Test_Org",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      expect(json.organization_slug).toBe("gh/my_test_org");
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
    // Slug collision tests are complex because the optimized generateUniqueSlug
    // uses a single batch query. The basic flow tests above cover the primary path.
    // These tests verify the slug suffix logic works correctly.

    it("appends suffix when slug already exists", async () => {
      // First query: no existing organization with this installation
      mockLimit.mockResolvedValueOnce([]);

      // Create a mock for the optimized slug query that returns one conflict
      let queryCount = 0;
      mockWhere.mockImplementation(() => {
        queryCount++;
        if (queryCount === 1) {
          // First call: check for existing installation (no conflict)
          return { limit: mockLimit };
        }
        // Second call: slug lookup returns one existing slug
        return Promise.resolve([{ slug: "gh/test-org" }]);
      });

      const payload = createInstallationPayload("created", {
        accountLogin: "test-org",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      expect(json.organization_slug).toBe("gh/test-org-1");
    });

    it("increments suffix for multiple collisions", async () => {
      // First query: no existing organization with this installation
      mockLimit.mockResolvedValueOnce([]);

      let queryCount = 0;
      mockWhere.mockImplementation(() => {
        queryCount++;
        if (queryCount === 1) {
          return { limit: mockLimit };
        }
        // Return multiple existing slugs
        return Promise.resolve([
          { slug: "gh/popular-name" },
          { slug: "gh/popular-name-1" },
          { slug: "gh/popular-name-2" },
        ]);
      });

      const payload = createInstallationPayload("created", {
        accountLogin: "popular-name",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      expect(json.organization_slug).toBe("gh/popular-name-3");
    });

    it("falls back to UUID suffix after max attempts", async () => {
      // First query: no existing organization with this installation
      mockLimit.mockResolvedValueOnce([]);

      let queryCount = 0;
      mockWhere.mockImplementation(() => {
        queryCount++;
        if (queryCount === 1) {
          return { limit: mockLimit };
        }
        // All 11 potential slugs are taken
        return Promise.resolve([
          { slug: "gh/super-popular" },
          { slug: "gh/super-popular-1" },
          { slug: "gh/super-popular-2" },
          { slug: "gh/super-popular-3" },
          { slug: "gh/super-popular-4" },
          { slug: "gh/super-popular-5" },
          { slug: "gh/super-popular-6" },
          { slug: "gh/super-popular-7" },
          { slug: "gh/super-popular-8" },
          { slug: "gh/super-popular-9" },
          { slug: "gh/super-popular-10" },
        ]);
      });

      const payload = createInstallationPayload("created", {
        accountLogin: "super-popular",
      });

      const res = await makeWebhookRequest("installation", payload);
      const json = (await res.json()) as InstallationResponse;

      expect(res.status).toBe(200);
      // Falls back to UUID prefix (first 8 chars of mockUUID)
      expect(json.organization_slug).toBe("gh/super-popular-test-uui");
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

      expect(mockClientEnd).toHaveBeenCalled();
    });

    it("closes database connection on error", async () => {
      mockLimit.mockRejectedValueOnce(new Error("Test error"));

      const payload = createInstallationPayload("created");

      await makeWebhookRequest("installation", payload);

      expect(mockClientEnd).toHaveBeenCalled();
    });
  });

  describe("unknown installation actions", () => {
    it("ignores unknown action types", async () => {
      const payload = {
        action: "some_unknown_action",
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
        action: "some_unknown_action",
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

describe("webhooks - repository events", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Reset all mock implementations first
    mockSelect.mockReset();
    mockFrom.mockReset();
    mockWhere.mockReset();
    mockLimit.mockReset();
    mockUpdate.mockReset();
    mockSet.mockReset();

    // Setup mock chain for select queries - simpler version for these tests
    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });
    // Default: no project found
    mockLimit.mockResolvedValue([]);

    // Setup mock chain for update
    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: vi.fn().mockResolvedValue(undefined) });
  });

  const createRepositoryPayload = (
    action: "renamed" | "privatized" | "publicized" | "transferred",
    overrides: Partial<{
      repoId: number;
      repoName: string;
      repoFullName: string;
      isPrivate: boolean;
      installationId: number;
    }> = {}
  ) => ({
    action,
    repository: {
      id: overrides.repoId ?? 123_456_789,
      name: overrides.repoName ?? "my-repo",
      full_name: overrides.repoFullName ?? "test-org/my-repo",
      private: overrides.isPrivate ?? false,
    },
    installation: {
      id: overrides.installationId ?? 12_345_678,
    },
  });

  it("updates project when repository is renamed", async () => {
    // Mock finding the existing project
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-123",
        handle: "old-name",
        providerRepoName: "old-name",
        providerRepoFullName: "test-org/old-name",
        isPrivate: false,
      },
    ]);

    const payload = createRepositoryPayload("renamed", {
      repoName: "new-name",
      repoFullName: "test-org/new-name",
    });

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "repository renamed",
      project_id: "project-123",
      old_name: "test-org/old-name",
      new_name: "test-org/new-name",
    });

    expect(mockUpdate).toHaveBeenCalled();
    expect(mockSet).toHaveBeenCalledWith(
      expect.objectContaining({
        providerRepoName: "new-name",
        providerRepoFullName: "test-org/new-name",
      })
    );
  });

  it("updates project visibility when repository is privatized", async () => {
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-123",
        handle: "my-repo",
        providerRepoName: "my-repo",
        providerRepoFullName: "test-org/my-repo",
        isPrivate: false,
      },
    ]);

    const payload = createRepositoryPayload("privatized", { isPrivate: true });

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "repository privatized",
      project_id: "project-123",
      is_private: true,
    });

    expect(mockSet).toHaveBeenCalledWith(
      expect.objectContaining({
        isPrivate: true,
      })
    );
  });

  it("updates project visibility when repository is publicized", async () => {
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-123",
        handle: "my-repo",
        providerRepoName: "my-repo",
        providerRepoFullName: "test-org/my-repo",
        isPrivate: true,
      },
    ]);

    const payload = createRepositoryPayload("publicized", { isPrivate: false });

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "repository publicized",
      project_id: "project-123",
      is_private: false,
    });

    expect(mockSet).toHaveBeenCalledWith(
      expect.objectContaining({
        isPrivate: false,
      })
    );
  });

  it("returns project not found when repo ID does not match", async () => {
    mockLimit.mockResolvedValueOnce([]);

    const payload = createRepositoryPayload("renamed");

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "project not found",
      repo_id: 123_456_789,
    });

    expect(mockUpdate).not.toHaveBeenCalled();
  });

  it("ignores repository events without installation ID", async () => {
    const payload = {
      action: "renamed",
      repository: {
        id: 123_456_789,
        name: "my-repo",
        full_name: "test-org/my-repo",
        private: false,
      },
      // No installation field
    };

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "ignored",
      reason: "no installation",
    });
  });
});

describe("webhooks - new_permissions_accepted", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });
    mockLimit.mockResolvedValue([]);

    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: mockWhere });
  });

  it("updates organization updatedAt when permissions are accepted", async () => {
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
      message: "permissions updated",
      account: "test-org",
    });

    expect(mockUpdate).toHaveBeenCalled();
    expect(mockSet).toHaveBeenCalledWith(
      expect.objectContaining({
        updatedAt: expect.any(Date),
      })
    );
  });
});

describe("webhooks - installation.created with repositories", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });

    const createQueryResult = (result: unknown[] = []) => {
      const queryResult = Promise.resolve(result) as Promise<unknown[]> & {
        limit: ReturnType<typeof vi.fn>;
      };
      queryResult.limit = mockLimit;
      return queryResult;
    };

    mockWhere.mockImplementation(() => createQueryResult([]));
    mockLimit.mockResolvedValue([]);

    mockInsert.mockReturnValue({ values: mockValues });
    mockValues.mockReturnValue({
      onConflictDoNothing: mockOnConflictDoNothing,
    });
    mockOnConflictDoNothing.mockResolvedValue(undefined);
  });

  it("creates organization with provider-prefixed slug (gh/login)", async () => {
    const payload = {
      action: "created",
      installation: {
        id: 12_345_678,
        account: {
          id: 98_765_432,
          login: "MyOrganization",
          type: "Organization" as const,
          avatar_url: "https://avatars.example.com/u/123",
        },
      },
      repositories: [],
    };

    const res = await makeWebhookRequest("installation", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json.organization_slug).toBe("gh/myorganization");
    expect(mockValues).toHaveBeenCalledWith(
      expect.objectContaining({
        slug: "gh/myorganization",
        provider: "github",
      })
    );
  });

  it("creates projects for all repositories in payload", async () => {
    const payload = {
      action: "created",
      installation: {
        id: 12_345_678,
        account: {
          id: 98_765_432,
          login: "test-org",
          type: "Organization" as const,
        },
      },
      repositories: [
        {
          id: 1001,
          name: "repo-one",
          full_name: "test-org/repo-one",
          private: false,
        },
        {
          id: 1002,
          name: "Repo-Two",
          full_name: "test-org/Repo-Two",
          private: true,
        },
        {
          id: 1003,
          name: "REPO-THREE",
          full_name: "test-org/REPO-THREE",
          private: false,
        },
      ],
    };

    const res = await makeWebhookRequest("installation", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json.projects_created).toBe(3);

    // Verify projects are created with correct values
    expect(mockInsert).toHaveBeenCalledTimes(2); // Once for org, once for projects
    expect(mockValues).toHaveBeenCalledWith([
      expect.objectContaining({
        handle: "repo-one", // lowercase
        providerRepoId: "1001",
        providerRepoName: "repo-one",
        providerRepoFullName: "test-org/repo-one",
        isPrivate: false,
      }),
      expect.objectContaining({
        handle: "repo-two", // lowercase
        providerRepoId: "1002",
        providerRepoName: "Repo-Two", // preserves original case in repo name
        providerRepoFullName: "test-org/Repo-Two",
        isPrivate: true,
      }),
      expect.objectContaining({
        handle: "repo-three", // lowercase
        providerRepoId: "1003",
        providerRepoName: "REPO-THREE",
        providerRepoFullName: "test-org/REPO-THREE",
        isPrivate: false,
      }),
    ]);
  });

  it("creates project handles as lowercase repo names", async () => {
    const payload = {
      action: "created",
      installation: {
        id: 12_345_678,
        account: {
          id: 98_765_432,
          login: "test-org",
          type: "Organization" as const,
        },
      },
      repositories: [
        {
          id: 1001,
          name: "My-Mixed-Case-Repo",
          full_name: "test-org/My-Mixed-Case-Repo",
          private: false,
        },
      ],
    };

    const res = await makeWebhookRequest("installation", payload);

    expect(res.status).toBe(200);
    expect(mockValues).toHaveBeenCalledWith([
      expect.objectContaining({
        handle: "my-mixed-case-repo",
        providerRepoName: "My-Mixed-Case-Repo",
      }),
    ]);
  });
});

describe("webhooks - installation.deleted data integrity", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });
    mockLimit.mockResolvedValue([]);

    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: vi.fn().mockResolvedValue(undefined) });
  });

  it("soft-deletes organization (sets deletedAt, does not hard delete)", async () => {
    const payload = {
      action: "deleted",
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
    expect(json.message).toBe("installation deleted");

    // Verify update (soft-delete) was called, not delete
    expect(mockUpdate).toHaveBeenCalled();
    expect(mockSet).toHaveBeenCalledWith(
      expect.objectContaining({
        deletedAt: expect.any(Date),
        updatedAt: expect.any(Date),
      })
    );
  });
});

describe("webhooks - repository renamed (critical)", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });

    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: vi.fn().mockResolvedValue(undefined) });
  });

  it("updates providerRepoName and providerRepoFullName on rename", async () => {
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-uuid-123",
        handle: "original-handle",
        providerRepoName: "old-repo-name",
        providerRepoFullName: "test-org/old-repo-name",
        isPrivate: false,
      },
    ]);

    const payload = {
      action: "renamed",
      repository: {
        id: 999_888,
        name: "new-repo-name",
        full_name: "test-org/new-repo-name",
        private: false,
      },
      installation: { id: 12_345_678 },
    };

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "repository renamed",
      project_id: "project-uuid-123",
      old_name: "test-org/old-repo-name",
      new_name: "test-org/new-repo-name",
    });

    expect(mockSet).toHaveBeenCalledWith(
      expect.objectContaining({
        providerRepoName: "new-repo-name",
        providerRepoFullName: "test-org/new-repo-name",
        updatedAt: expect.any(Date),
      })
    );
  });

  it("preserves project handle when repository is renamed (critical)", async () => {
    // User may have customized their project handle - renaming repo should NOT change it
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-uuid-456",
        handle: "my-custom-handle", // Custom handle that differs from repo name
        providerRepoName: "old-name",
        providerRepoFullName: "test-org/old-name",
        isPrivate: false,
      },
    ]);

    const payload = {
      action: "renamed",
      repository: {
        id: 123_456,
        name: "new-name",
        full_name: "test-org/new-name",
        private: false,
      },
      installation: { id: 12_345_678 },
    };

    const res = await makeWebhookRequest("repository", payload);

    expect(res.status).toBe(200);

    // The update should NOT include handle - handle must be preserved
    expect(mockSet).toHaveBeenCalledWith({
      providerRepoName: "new-name",
      providerRepoFullName: "test-org/new-name",
      updatedAt: expect.any(Date),
    });

    // Verify handle is NOT in the update call
    const setCallArgs = mockSet.mock.calls[0][0];
    expect(setCallArgs).not.toHaveProperty("handle");
  });

  it("preserves project ID on rename (no data loss)", async () => {
    const existingProjectId = "project-uuid-preserved";

    mockLimit.mockResolvedValueOnce([
      {
        id: existingProjectId,
        handle: "my-project",
        providerRepoName: "old-name",
        providerRepoFullName: "test-org/old-name",
        isPrivate: false,
      },
    ]);

    const payload = {
      action: "renamed",
      repository: {
        id: 777_888,
        name: "completely-new-name",
        full_name: "test-org/completely-new-name",
        private: false,
      },
      installation: { id: 12_345_678 },
    };

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    // Response confirms same project ID was updated
    expect(json.project_id).toBe(existingProjectId);

    // Update was called (not insert - which would create new record)
    expect(mockUpdate).toHaveBeenCalled();
    expect(mockInsert).not.toHaveBeenCalled();
  });
});

describe("webhooks - repository transferred", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });

    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: vi.fn().mockResolvedValue(undefined) });
  });

  it("updates providerRepoFullName on transfer", async () => {
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-uuid-transfer",
        handle: "my-project",
        providerRepoName: "my-repo",
        providerRepoFullName: "old-org/my-repo",
        isPrivate: false,
      },
    ]);

    const payload = {
      action: "transferred",
      repository: {
        id: 555_666,
        name: "my-repo",
        full_name: "new-org/my-repo", // Transferred to new org
        private: false,
      },
      installation: { id: 12_345_678 },
    };

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "repository transferred",
      project_id: "project-uuid-transfer",
      new_full_name: "new-org/my-repo",
    });

    expect(mockSet).toHaveBeenCalledWith({
      providerRepoFullName: "new-org/my-repo",
      updatedAt: expect.any(Date),
    });
  });

  it("keeps project linked to original organization after transfer", async () => {
    const originalProjectId = "project-stays-with-org";

    mockLimit.mockResolvedValueOnce([
      {
        id: originalProjectId,
        handle: "transferred-project",
        providerRepoName: "repo",
        providerRepoFullName: "original-org/repo",
        isPrivate: false,
      },
    ]);

    const payload = {
      action: "transferred",
      repository: {
        id: 111_222,
        name: "repo",
        full_name: "different-org/repo",
        private: false,
      },
      installation: { id: 12_345_678 },
    };

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json.project_id).toBe(originalProjectId);

    // Only providerRepoFullName updated, not organizationId
    const setCallArgs = mockSet.mock.calls[0][0];
    expect(setCallArgs).not.toHaveProperty("organizationId");
  });
});

describe("webhooks - repository visibility changed", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });

    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: vi.fn().mockResolvedValue(undefined) });
  });

  it("updates isPrivate to true when privatized", async () => {
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-visibility",
        handle: "public-repo",
        providerRepoName: "public-repo",
        providerRepoFullName: "test-org/public-repo",
        isPrivate: false,
      },
    ]);

    const payload = {
      action: "privatized",
      repository: {
        id: 333_444,
        name: "public-repo",
        full_name: "test-org/public-repo",
        private: true,
      },
      installation: { id: 12_345_678 },
    };

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "repository privatized",
      project_id: "project-visibility",
      is_private: true,
    });

    expect(mockSet).toHaveBeenCalledWith({
      isPrivate: true,
      updatedAt: expect.any(Date),
    });
  });

  it("updates isPrivate to false when publicized", async () => {
    mockLimit.mockResolvedValueOnce([
      {
        id: "project-visibility-2",
        handle: "private-repo",
        providerRepoName: "private-repo",
        providerRepoFullName: "test-org/private-repo",
        isPrivate: true,
      },
    ]);

    const payload = {
      action: "publicized",
      repository: {
        id: 444_555,
        name: "private-repo",
        full_name: "test-org/private-repo",
        private: false,
      },
      installation: { id: 12_345_678 },
    };

    const res = await makeWebhookRequest("repository", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "repository publicized",
      project_id: "project-visibility-2",
      is_private: false,
    });

    expect(mockSet).toHaveBeenCalledWith({
      isPrivate: false,
      updatedAt: expect.any(Date),
    });
  });
});

describe("webhooks - installation_repositories (add/remove)", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockReturnValue({ limit: mockLimit });

    mockInsert.mockReturnValue({ values: mockValues });
    mockValues.mockReturnValue({
      onConflictDoNothing: mockOnConflictDoNothing,
    });
    mockOnConflictDoNothing.mockResolvedValue(undefined);

    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: vi.fn().mockResolvedValue(undefined) });
  });

  it("creates new projects when repositories are added", async () => {
    // Mock finding the organization
    mockLimit.mockResolvedValueOnce([
      { id: "org-uuid-123", slug: "gh/test-org" },
    ]);

    const payload = {
      action: "added",
      installation: {
        id: 12_345_678,
        account: {
          id: 98_765_432,
          login: "test-org",
          type: "Organization" as const,
        },
      },
      repositories_added: [
        {
          id: 2001,
          name: "new-repo",
          full_name: "test-org/new-repo",
          private: false,
        },
        {
          id: 2002,
          name: "Another-Repo",
          full_name: "test-org/Another-Repo",
          private: true,
        },
      ],
      repositories_removed: [],
    };

    const res = await makeWebhookRequest("installation_repositories", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "installation_repositories processed",
      organization_id: "org-uuid-123",
      organization_slug: "gh/test-org",
      projects_added: 2,
      projects_removed: 0,
    });

    expect(mockValues).toHaveBeenCalledWith([
      expect.objectContaining({
        organizationId: "org-uuid-123",
        handle: "new-repo",
        providerRepoId: "2001",
        providerRepoName: "new-repo",
        isPrivate: false,
      }),
      expect.objectContaining({
        organizationId: "org-uuid-123",
        handle: "another-repo", // lowercase
        providerRepoId: "2002",
        providerRepoName: "Another-Repo",
        isPrivate: true,
      }),
    ]);
  });

  it("soft-deletes projects when repositories are removed (sets removedAt)", async () => {
    mockLimit.mockResolvedValueOnce([
      { id: "org-uuid-456", slug: "gh/test-org" },
    ]);

    const payload = {
      action: "removed",
      installation: {
        id: 12_345_678,
        account: {
          id: 98_765_432,
          login: "test-org",
          type: "Organization" as const,
        },
      },
      repositories_added: [],
      repositories_removed: [
        {
          id: 3001,
          name: "removed-repo",
          full_name: "test-org/removed-repo",
          private: false,
        },
        {
          id: 3002,
          name: "also-removed",
          full_name: "test-org/also-removed",
          private: true,
        },
      ],
    };

    const res = await makeWebhookRequest("installation_repositories", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json.projects_removed).toBe(2);

    // Verify soft-delete (update with removedAt) not hard delete
    expect(mockUpdate).toHaveBeenCalled();
    expect(mockSet).toHaveBeenCalledWith({
      removedAt: expect.any(Date),
      updatedAt: expect.any(Date),
    });
  });

  it("handles both added and removed in same event", async () => {
    mockLimit.mockResolvedValueOnce([
      { id: "org-uuid-789", slug: "gh/test-org" },
    ]);

    const payload = {
      action: "added",
      installation: {
        id: 12_345_678,
        account: {
          id: 98_765_432,
          login: "test-org",
          type: "Organization" as const,
        },
      },
      repositories_added: [
        {
          id: 4001,
          name: "added-repo",
          full_name: "test-org/added-repo",
          private: false,
        },
      ],
      repositories_removed: [
        {
          id: 4002,
          name: "removed-repo",
          full_name: "test-org/removed-repo",
          private: false,
        },
      ],
    };

    const res = await makeWebhookRequest("installation_repositories", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json.projects_added).toBe(1);
    expect(json.projects_removed).toBe(1);

    // Both insert (for added) and update (for removed) should be called
    expect(mockInsert).toHaveBeenCalled();
    expect(mockUpdate).toHaveBeenCalled();
  });

  it("returns organization not found when installation has no org", async () => {
    mockLimit.mockResolvedValueOnce([]); // No org found

    const payload = {
      action: "added",
      installation: {
        id: 99_999_999, // Unknown installation
        account: {
          id: 11_111_111,
          login: "unknown-org",
          type: "Organization" as const,
        },
      },
      repositories_added: [
        {
          id: 5001,
          name: "repo",
          full_name: "unknown-org/repo",
          private: false,
        },
      ],
      repositories_removed: [],
    };

    const res = await makeWebhookRequest("installation_repositories", payload);
    const json = await res.json();

    expect(res.status).toBe(200);
    expect(json).toEqual({
      message: "organization not found",
      installation_id: 99_999_999,
    });

    // No insert/update should happen
    expect(mockInsert).not.toHaveBeenCalled();
    expect(mockValues).not.toHaveBeenCalled();
  });
});
