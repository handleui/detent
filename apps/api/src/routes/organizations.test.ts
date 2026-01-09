import { Hono } from "hono";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Env } from "../types/env";

// Mock the GitHub service
const mockGetInstallationInfo = vi.fn();
const mockGetInstallationRepos = vi.fn();

vi.mock("../services/github", () => ({
  createGitHubService: vi.fn(() => ({
    getInstallationInfo: mockGetInstallationInfo,
    getInstallationRepos: mockGetInstallationRepos,
  })),
}));

// Mock the database client
const mockFindFirst = vi.fn();
const mockSelect = vi.fn();
const mockFrom = vi.fn();
const mockWhere = vi.fn();
const mockInsert = vi.fn();
const mockValues = vi.fn();
const mockUpdate = vi.fn();
const mockSet = vi.fn();

const mockDb = {
  query: {
    organizationMembers: {
      findFirst: mockFindFirst,
    },
  },
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

// Test UUIDs (valid UUID v4 format)
const TEST_ORG_ID = "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d";
const TEST_PROJECT_ID = "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e";
const TEST_MEMBER_ID = "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f";
const NEW_PROJECT_UUID = "d4e5f6a7-b8c9-4d0e-1f2a-3b4c5d6e7f8a";

// Mock crypto.randomUUID for deterministic IDs
vi.spyOn(crypto, "randomUUID").mockImplementation(() => NEW_PROJECT_UUID);

// Mock environment
const MOCK_ENV = {
  GITHUB_APP_ID: "123456",
  GITHUB_APP_PRIVATE_KEY: "test-private-key",
  HYPERDRIVE: {
    connectionString: "postgres://test:test@localhost:5432/test",
  },
  WORKOS_CLIENT_ID: "test-workos-client",
  WORKOS_API_KEY: "test-workos-key",
};

// Factory for organization data
const createOrganization = (
  overrides: Partial<{
    id: string;
    name: string;
    slug: string;
    provider: "github" | "gitlab";
    providerInstallationId: string | null;
    suspendedAt: Date | null;
    deletedAt: Date | null;
    lastSyncedAt: Date | null;
  }> = {}
) => ({
  id: overrides.id ?? TEST_ORG_ID,
  name: overrides.name ?? "test-org",
  slug: overrides.slug ?? "gh/test-org",
  provider: overrides.provider ?? "github",
  providerAccountId: "12345",
  providerAccountLogin: "test-org",
  providerAccountType: "organization" as const,
  providerInstallationId:
    "providerInstallationId" in overrides
      ? overrides.providerInstallationId
      : "inst-123",
  suspendedAt: "suspendedAt" in overrides ? overrides.suspendedAt : null,
  deletedAt: "deletedAt" in overrides ? overrides.deletedAt : null,
  lastSyncedAt: "lastSyncedAt" in overrides ? overrides.lastSyncedAt : null,
  allowAutoJoin: true,
  createdAt: new Date("2024-01-01"),
  updatedAt: new Date("2024-01-01"),
});

// Factory for organization member
const createMember = (
  role: "owner" | "admin" | "member" = "owner",
  organization = createOrganization()
) => ({
  id: TEST_MEMBER_ID,
  organizationId: organization.id,
  userId: "user-123",
  role,
  organization,
});

// Factory for project data
const createProject = (
  overrides: Partial<{
    id: string;
    providerRepoId: string;
    providerRepoName: string;
    providerRepoFullName: string;
    handle: string;
    isPrivate: boolean;
    removedAt: Date | null;
  }> = {}
) => ({
  id: overrides.id ?? TEST_PROJECT_ID,
  organizationId: TEST_ORG_ID,
  providerRepoId: overrides.providerRepoId ?? "repo-123",
  providerRepoName: overrides.providerRepoName ?? "my-repo",
  providerRepoFullName: overrides.providerRepoFullName ?? "test-org/my-repo",
  handle: overrides.handle ?? "my-repo",
  isPrivate: overrides.isPrivate ?? false,
  removedAt: overrides.removedAt ?? null,
});

// Factory for GitHub repo data from API
const createGitHubRepo = (
  overrides: Partial<{
    id: number;
    name: string;
    full_name: string;
    private: boolean;
    default_branch: string;
  }> = {}
) => ({
  id: overrides.id ?? 123,
  name: overrides.name ?? "my-repo",
  full_name: overrides.full_name ?? "test-org/my-repo",
  private: overrides.private ?? false,
  default_branch: overrides.default_branch ?? "main",
});

// Helper to make request
const makeRequest = async (
  method: "GET" | "POST",
  path: string,
  auth: { userId: string; role?: "owner" | "admin" | "member" } = {
    userId: "user-123",
    role: "owner",
  }
): Promise<Response> => {
  const organizationRoutes = (await import("./organizations")).default;

  const app = new Hono<{ Bindings: Env }>();

  // Middleware to set auth context
  app.use("*", async (c, next) => {
    c.set("auth" as never, { userId: auth.userId } as never);
    await next();
  });

  app.route("/organizations", organizationRoutes);

  return app.request(path, { method }, MOCK_ENV);
};

// Response types
interface SyncResponse {
  message: string;
  organization_id?: string;
  suspended?: boolean;
  projects_added?: number;
  projects_removed?: number;
  projects_updated?: number;
  total_repos?: number;
  synced?: boolean;
  error?: string;
}

describe("organizations - POST /:organizationId/sync", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Reset all mock implementations
    mockFindFirst.mockReset();
    mockSelect.mockReset();
    mockFrom.mockReset();
    mockWhere.mockReset();
    mockInsert.mockReset();
    mockValues.mockReset();
    mockUpdate.mockReset();
    mockSet.mockReset();
    mockGetInstallationInfo.mockReset();
    mockGetInstallationRepos.mockReset();

    // Setup mock chain for select
    mockSelect.mockReturnValue({ from: mockFrom });
    mockFrom.mockReturnValue({ where: mockWhere });
    mockWhere.mockResolvedValue([]);

    // Setup mock chain for insert
    mockInsert.mockReturnValue({ values: mockValues });
    mockValues.mockResolvedValue(undefined);

    // Setup mock chain for update
    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: mockWhere });
  });

  describe("authorization", () => {
    it("returns 403 when user is not a member of the organization", async () => {
      mockFindFirst.mockResolvedValue(undefined);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`,
        { userId: "non-member-user" }
      );
      const json = await res.json();

      expect(res.status).toBe(403);
      expect(json).toEqual({ error: "Not a member of this organization" });
    });

    it("returns 403 when user has member role (not admin or owner)", async () => {
      mockFindFirst.mockResolvedValue(createMember("member"));

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`,
        { userId: "user-123", role: "member" }
      );
      const json = await res.json();

      expect(res.status).toBe(403);
      expect(json).toEqual({
        error: "Only organization owners and admins can trigger sync",
      });
    });

    it("allows admin to trigger sync", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("admin", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });
      mockGetInstallationRepos.mockResolvedValue([]);
      mockWhere.mockResolvedValue([]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`,
        { userId: "user-123", role: "admin" }
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.message).toBe("sync completed");
    });

    it("allows owner to trigger sync", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });
      mockGetInstallationRepos.mockResolvedValue([]);
      mockWhere.mockResolvedValue([]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`,
        { userId: "user-123", role: "owner" }
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.message).toBe("sync completed");
    });
  });

  describe("installation removed detection", () => {
    it("marks organization as deleted when GitHub returns 404 for installation", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue(null);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json).toEqual({
        message: "installation_removed",
        organization_id: TEST_ORG_ID,
        synced: true,
      });

      // Verify deletedAt and lastSyncedAt were both set
      expect(mockUpdate).toHaveBeenCalled();
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          deletedAt: expect.any(Date),
          lastSyncedAt: expect.any(Date),
          updatedAt: expect.any(Date),
        })
      );
    });

    it("updates lastSyncedAt even when installation is removed", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue(null);

      await makeRequest("POST", `/organizations/${TEST_ORG_ID}/sync`);

      // Verify that lastSyncedAt is specifically set
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          lastSyncedAt: expect.any(Date),
        })
      );
    });
  });

  describe("suspension status sync", () => {
    it("sets suspendedAt when GitHub shows suspended", async () => {
      const org = createOrganization({ suspendedAt: null });
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: "2024-06-15T10:00:00Z",
        account: { login: "test-org" },
      });
      mockGetInstallationRepos.mockResolvedValue([]);
      mockWhere.mockResolvedValue([]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.suspended).toBe(true);

      // Verify suspendedAt was set
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          suspendedAt: expect.any(Date),
        })
      );
    });

    it("clears suspendedAt when GitHub shows unsuspended", async () => {
      const org = createOrganization({ suspendedAt: new Date("2024-06-01") });
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });
      mockGetInstallationRepos.mockResolvedValue([]);
      mockWhere.mockResolvedValue([]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.suspended).toBe(false);

      // Verify suspendedAt was cleared
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          suspendedAt: null,
        })
      );
    });

    it("does not update when suspension status is unchanged", async () => {
      const org = createOrganization({ suspendedAt: null });
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });
      mockGetInstallationRepos.mockResolvedValue([]);
      mockWhere.mockResolvedValue([]);

      await makeRequest("POST", `/organizations/${TEST_ORG_ID}/sync`);

      // The first update call should NOT contain suspendedAt (only lastSyncedAt at the end)
      // Check that suspendedAt: null is not explicitly set when already null
      const suspensionUpdateCall = mockSet.mock.calls.find(
        (call) => "suspendedAt" in call[0]
      );
      expect(suspensionUpdateCall).toBeUndefined();
    });
  });

  describe("project reconciliation", () => {
    it("creates new projects for repos in GitHub but not in Detent", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // GitHub has 2 repos, Detent has none
      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 100,
          name: "repo-a",
          full_name: "test-org/repo-a",
        }),
        createGitHubRepo({
          id: 200,
          name: "repo-b",
          full_name: "test-org/repo-b",
        }),
      ]);

      // No existing projects
      mockWhere.mockResolvedValue([]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.projects_added).toBe(2);
      expect(json.total_repos).toBe(2);

      // Verify insert was called for new projects
      expect(mockInsert).toHaveBeenCalled();
    });

    it("soft-deletes projects in Detent but no longer in GitHub", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // GitHub has no repos
      mockGetInstallationRepos.mockResolvedValue([]);

      // Detent has 2 active projects
      mockWhere.mockResolvedValue([
        createProject({
          id: "proj-1",
          providerRepoId: "100",
          providerRepoName: "repo-a",
        }),
        createProject({
          id: "proj-2",
          providerRepoId: "200",
          providerRepoName: "repo-b",
        }),
      ]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.projects_removed).toBe(2);

      // Verify update was called with removedAt
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          removedAt: expect.any(Date),
        })
      );
    });

    it("reactivates previously soft-deleted repos that reappear in GitHub", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // GitHub has the repo again
      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 100,
          name: "restored-repo",
          full_name: "test-org/restored-repo",
        }),
      ]);

      // Detent has the project but it was soft-deleted
      mockWhere.mockResolvedValue([
        createProject({
          id: "proj-1",
          providerRepoId: "100",
          providerRepoName: "restored-repo",
          removedAt: new Date("2024-05-01"),
        }),
      ]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      // Reactivated projects count as "updated", not "added"
      expect(json.projects_updated).toBeGreaterThanOrEqual(1);

      // Verify removedAt was cleared (set to null)
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          removedAt: null,
        })
      );
    });

    it("preserves project ID when reactivating soft-deleted project", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      const existingProjectId = "existing-project-id-preserved";

      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 100,
          name: "my-repo",
          full_name: "test-org/my-repo",
        }),
      ]);

      mockWhere.mockResolvedValue([
        createProject({
          id: existingProjectId,
          providerRepoId: "100",
          removedAt: new Date("2024-05-01"),
        }),
      ]);

      await makeRequest("POST", `/organizations/${TEST_ORG_ID}/sync`);

      // Verify we used UPDATE (not INSERT) to preserve the ID
      const insertCalls = mockInsert.mock.calls.length;
      // Insert should NOT be called for reactivation
      // The update should target the existing project
      expect(mockUpdate).toHaveBeenCalled();
      // No new projects should be inserted since we're reactivating
      expect(insertCalls).toBe(0);
    });
  });

  describe("project updates during sync", () => {
    it("updates project when repo name changes", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // GitHub has repo with new name
      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 100,
          name: "new-name",
          full_name: "test-org/new-name",
        }),
      ]);

      // Detent has project with old name
      mockWhere.mockResolvedValue([
        createProject({
          id: "proj-1",
          providerRepoId: "100",
          providerRepoName: "old-name",
          providerRepoFullName: "test-org/old-name",
        }),
      ]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.projects_updated).toBeGreaterThanOrEqual(1);

      // Verify providerRepoName was updated
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          providerRepoName: "new-name",
          providerRepoFullName: "test-org/new-name",
        })
      );
    });

    it("updates project when visibility changes", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // GitHub repo is now private
      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 100,
          name: "my-repo",
          full_name: "test-org/my-repo",
          private: true,
        }),
      ]);

      // Detent has it as public
      mockWhere.mockResolvedValue([
        createProject({
          id: "proj-1",
          providerRepoId: "100",
          isPrivate: false,
        }),
      ]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.projects_updated).toBeGreaterThanOrEqual(1);

      // Verify isPrivate was updated
      expect(mockSet).toHaveBeenCalledWith(
        expect.objectContaining({
          isPrivate: true,
        })
      );
    });

    it("does NOT update project handle when repo name changes", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // GitHub has repo with new name
      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 100,
          name: "new-repo-name",
          full_name: "test-org/new-repo-name",
        }),
      ]);

      // Detent has project with custom handle
      mockWhere.mockResolvedValue([
        createProject({
          id: "proj-1",
          providerRepoId: "100",
          providerRepoName: "old-repo-name",
          handle: "custom-handle",
        }),
      ]);

      await makeRequest("POST", `/organizations/${TEST_ORG_ID}/sync`);

      // Verify handle was NOT included in any update
      const allSetCalls = mockSet.mock.calls;
      for (const call of allSetCalls) {
        const updateFields = call[0];
        expect(updateFields).not.toHaveProperty("handle");
      }
    });
  });

  describe("edge cases", () => {
    it("returns 404 when organization has been deleted", async () => {
      const org = createOrganization({ deletedAt: new Date("2024-06-01") });
      mockFindFirst.mockResolvedValue(createMember("owner", org));

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = await res.json();

      expect(res.status).toBe(404);
      expect(json).toEqual({ error: "Organization has been deleted" });
    });

    it("returns 400 for GitLab organizations", async () => {
      const org = createOrganization({ provider: "gitlab" });
      mockFindFirst.mockResolvedValue(createMember("owner", org));

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = await res.json();

      expect(res.status).toBe(400);
      expect(json).toEqual({
        error: "Sync only supported for GitHub organizations",
      });
    });

    it("returns 400 when no GitHub App installation exists", async () => {
      const org = createOrganization({ providerInstallationId: null });
      mockFindFirst.mockResolvedValue(createMember("owner", org));

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = await res.json();

      expect(res.status).toBe(400);
      expect(json).toEqual({ error: "No GitHub App installation found" });
    });

    it("returns 400 for invalid organization ID format", async () => {
      const res = await makeRequest("POST", "/organizations/invalid-id/sync");
      const json = await res.json();

      expect(res.status).toBe(400);
      expect(json).toHaveProperty("error");
    });

    it("closes database connection on success", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });
      mockGetInstallationRepos.mockResolvedValue([]);
      mockWhere.mockResolvedValue([]);

      await makeRequest("POST", `/organizations/${TEST_ORG_ID}/sync`);

      expect(mockClient.end).toHaveBeenCalled();
    });

    it("closes database connection on error", async () => {
      mockFindFirst.mockRejectedValue(new Error("Database error"));

      await makeRequest("POST", `/organizations/${TEST_ORG_ID}/sync`);

      expect(mockClient.end).toHaveBeenCalled();
    });

    it("returns 500 on GitHub API error", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockRejectedValue(
        new Error("GitHub API rate limit exceeded")
      );

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(500);
      expect(json.message).toBe("sync error");
      expect(json.error).toBe("GitHub API rate limit exceeded");
    });
  });

  describe("data integrity", () => {
    it("handles mixed scenario: add, remove, update in single sync", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // GitHub has: repo-a (existing, renamed), repo-c (new)
      // Missing: repo-b (was in Detent)
      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 100,
          name: "repo-a-renamed",
          full_name: "test-org/repo-a-renamed",
        }),
        createGitHubRepo({
          id: 300,
          name: "repo-c",
          full_name: "test-org/repo-c",
        }),
      ]);

      // Detent has: repo-a (old name), repo-b (will be removed)
      mockWhere.mockResolvedValue([
        createProject({
          id: "proj-a",
          providerRepoId: "100",
          providerRepoName: "repo-a",
          providerRepoFullName: "test-org/repo-a",
        }),
        createProject({
          id: "proj-b",
          providerRepoId: "200",
          providerRepoName: "repo-b",
          providerRepoFullName: "test-org/repo-b",
        }),
      ]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      expect(json.projects_added).toBe(1); // repo-c
      expect(json.projects_removed).toBe(1); // repo-b
      expect(json.projects_updated).toBe(1); // repo-a renamed
      expect(json.total_repos).toBe(2);
    });

    it("correctly identifies repos by providerRepoId not by name", async () => {
      const org = createOrganization();
      mockFindFirst.mockResolvedValue(createMember("owner", org));
      mockGetInstallationInfo.mockResolvedValue({
        id: 123,
        suspended_at: null,
        account: { login: "test-org" },
      });

      // Same name "my-repo" but different ID - should be treated as new
      mockGetInstallationRepos.mockResolvedValue([
        createGitHubRepo({
          id: 999,
          name: "my-repo",
          full_name: "test-org/my-repo",
        }),
      ]);

      // Existing project with same name but different repo ID
      mockWhere.mockResolvedValue([
        createProject({
          id: "proj-1",
          providerRepoId: "100",
          providerRepoName: "my-repo",
        }),
      ]);

      const res = await makeRequest(
        "POST",
        `/organizations/${TEST_ORG_ID}/sync`
      );
      const json = (await res.json()) as SyncResponse;

      expect(res.status).toBe(200);
      // Old project (repo ID 100) should be removed
      expect(json.projects_removed).toBe(1);
      // New project (repo ID 999) should be added
      expect(json.projects_added).toBe(1);
    });
  });
});
