import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Organization, OrganizationMember } from "../db/schema";

// Separate mocks for each table
const mockOrgFindFirst = vi.fn();
const mockMemberFindFirst = vi.fn();
const mockUpdate = vi.fn();
const mockInsert = vi.fn();
const mockSet = vi.fn();
const mockWhere = vi.fn();
const mockValues = vi.fn();

const mockDb = {
  query: {
    organizations: { findFirst: mockOrgFindFirst },
    organizationMembers: { findFirst: mockMemberFindFirst },
  },
  update: mockUpdate,
  insert: mockInsert,
};

const mockClient = { end: vi.fn() };

vi.mock("../db/client", () => ({
  createDb: vi.fn(() => Promise.resolve({ db: mockDb, client: mockClient })),
}));

// Mock GitHub identity verification
const mockGetVerifiedGitHubIdentity = vi.fn();
vi.mock("../lib/github-identity", () => ({
  getVerifiedGitHubIdentity: (...args: unknown[]) =>
    mockGetVerifiedGitHubIdentity(...args),
}));

// Mock GitHub membership verification
const mockVerifyGitHubMembership = vi.fn();
vi.mock("../lib/github-membership", () => ({
  verifyGitHubMembership: (...args: unknown[]) =>
    mockVerifyGitHubMembership(...args),
}));

// Factory helpers
const createOrg = (overrides: Partial<Organization> = {}): Organization => ({
  id: "org-123",
  name: "Test Org",
  slug: "gh/test-org",
  enterpriseId: null,
  provider: "github",
  providerAccountId: "123456",
  providerAccountLogin: "test-org",
  providerAccountType: "organization",
  providerAvatarUrl: null,
  providerInstallationId: "inst-789",
  providerAccessTokenEncrypted: null,
  providerAccessTokenExpiresAt: null,
  providerWebhookSecret: null,
  installerGithubId: "installer-999",
  suspendedAt: null,
  deletedAt: null,
  lastSyncedAt: null,
  allowAutoJoin: true,
  createdAt: new Date(),
  updatedAt: new Date(),
  ...overrides,
});

const createMember = (
  overrides: Partial<OrganizationMember> = {}
): OrganizationMember => ({
  id: "member-123",
  organizationId: "org-123",
  userId: "user-abc",
  role: "member",
  providerUserId: "gh-user-456",
  providerUsername: "testuser",
  providerLinkedAt: new Date(),
  createdAt: new Date(),
  updatedAt: new Date(),
  ...overrides,
});

const MOCK_ENV = {
  WORKOS_API_KEY: "sk_test_workos_key",
  WORKOS_CLIENT_ID: "client_123",
  HYPERDRIVE: { connectionString: "postgres://test:test@localhost:5432/test" },
  GITHUB_APP_ID: "123",
  GITHUB_PRIVATE_KEY: "key",
};

describe("github-org-access middleware", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUpdate.mockReturnValue({ set: mockSet });
    mockSet.mockReturnValue({ where: mockWhere });
    mockWhere.mockResolvedValue([]);
    mockInsert.mockReturnValue({ values: mockValues });
    mockValues.mockResolvedValue([]);
  });

  describe("seedRoleFromGitHub - role seeding for new members", () => {
    // We test the role seeding logic by verifying what role gets inserted
    // when a new member is created

    it("installer gets owner role even if GitHub says member", async () => {
      const org = createOrg({ installerGithubId: "installer-999" });

      mockOrgFindFirst.mockResolvedValue(org);
      mockMemberFindFirst.mockResolvedValue(undefined); // No existing member

      mockGetVerifiedGitHubIdentity.mockResolvedValue({
        userId: "installer-999",
        username: "installer-user",
      });

      mockVerifyGitHubMembership.mockResolvedValue({
        isMember: true,
        role: "member", // GitHub says "member"
      });

      const { githubOrgAccessMiddleware } = await import("./github-org-access");
      const { Hono } = await import("hono");
      const app = new Hono();

      app.use("*", async (c, next) => {
        c.set("auth" as never, { userId: "user-abc" } as never);
        await next();
      });
      app.use("/orgs/:orgId/*", githubOrgAccessMiddleware);
      app.get("/orgs/:orgId/test", (c) => {
        const orgAccess = c.get("orgAccess");
        return c.json({ role: orgAccess.role });
      });

      const res = await app.request("/orgs/org-123/test", {}, MOCK_ENV);
      const json = (await res.json()) as { role: string };

      expect(res.status).toBe(200);
      expect(json.role).toBe("owner");

      // Verify role was seeded as "owner" in the insert
      expect(mockInsert).toHaveBeenCalled();
      const insertCall = mockValues.mock.calls[0]?.[0] as { role: string };
      expect(insertCall.role).toBe("owner");
    });

    it("GitHub admin gets admin role when not installer", async () => {
      const org = createOrg({ installerGithubId: "someone-else" });

      mockOrgFindFirst.mockResolvedValue(org);
      mockMemberFindFirst.mockResolvedValue(undefined);

      mockGetVerifiedGitHubIdentity.mockResolvedValue({
        userId: "admin-user-id",
        username: "admin-user",
      });

      mockVerifyGitHubMembership.mockResolvedValue({
        isMember: true,
        role: "admin",
      });

      const { githubOrgAccessMiddleware } = await import("./github-org-access");
      const { Hono } = await import("hono");
      const app = new Hono();

      app.use("*", async (c, next) => {
        c.set("auth" as never, { userId: "user-abc" } as never);
        await next();
      });
      app.use("/orgs/:orgId/*", githubOrgAccessMiddleware);
      app.get("/orgs/:orgId/test", (c) => {
        const orgAccess = c.get("orgAccess");
        return c.json({ role: orgAccess.role });
      });

      const res = await app.request("/orgs/org-123/test", {}, MOCK_ENV);
      const json = (await res.json()) as { role: string };

      expect(res.status).toBe(200);
      expect(json.role).toBe("admin");

      const insertCall = mockValues.mock.calls[0]?.[0] as { role: string };
      expect(insertCall.role).toBe("admin");
    });

    it("GitHub member gets member role", async () => {
      const org = createOrg({ installerGithubId: "someone-else" });

      mockOrgFindFirst.mockResolvedValue(org);
      mockMemberFindFirst.mockResolvedValue(undefined);

      mockGetVerifiedGitHubIdentity.mockResolvedValue({
        userId: "regular-user-id",
        username: "regular-user",
      });

      mockVerifyGitHubMembership.mockResolvedValue({
        isMember: true,
        role: "member",
      });

      const { githubOrgAccessMiddleware } = await import("./github-org-access");
      const { Hono } = await import("hono");
      const app = new Hono();

      app.use("*", async (c, next) => {
        c.set("auth" as never, { userId: "user-abc" } as never);
        await next();
      });
      app.use("/orgs/:orgId/*", githubOrgAccessMiddleware);
      app.get("/orgs/:orgId/test", (c) => {
        const orgAccess = c.get("orgAccess");
        return c.json({ role: orgAccess.role });
      });

      const res = await app.request("/orgs/org-123/test", {}, MOCK_ENV);
      const json = (await res.json()) as { role: string };

      expect(res.status).toBe(200);
      expect(json.role).toBe("member");

      const insertCall = mockValues.mock.calls[0]?.[0] as { role: string };
      expect(insertCall.role).toBe("member");
    });
  });

  describe("role decoupling - existing members use DB role", () => {
    it("existing admin keeps admin role even if GitHub says member", async () => {
      const org = createOrg();
      const member = createMember({ role: "admin" });

      mockOrgFindFirst.mockResolvedValue(org);
      mockMemberFindFirst.mockResolvedValue(member);

      mockGetVerifiedGitHubIdentity.mockResolvedValue({
        userId: "gh-user-456",
        username: "testuser",
      });

      mockVerifyGitHubMembership.mockResolvedValue({
        isMember: true,
        role: "member", // GitHub downgraded to member
      });

      const { githubOrgAccessMiddleware } = await import("./github-org-access");
      const { Hono } = await import("hono");
      const app = new Hono();

      app.use("*", async (c, next) => {
        c.set("auth" as never, { userId: "user-abc" } as never);
        await next();
      });
      app.use("/orgs/:orgId/*", githubOrgAccessMiddleware);
      app.get("/orgs/:orgId/test", (c) => {
        const orgAccess = c.get("orgAccess");
        return c.json({ role: orgAccess.role });
      });

      const res = await app.request("/orgs/org-123/test", {}, MOCK_ENV);
      const json = (await res.json()) as { role: string };

      expect(res.status).toBe(200);
      expect(json.role).toBe("admin"); // DB role preserved

      // No insert should happen - member already exists
      expect(mockInsert).not.toHaveBeenCalled();
    });

    it("existing member keeps member role even if GitHub says admin", async () => {
      const org = createOrg();
      const member = createMember({ role: "member" });

      mockOrgFindFirst.mockResolvedValue(org);
      mockMemberFindFirst.mockResolvedValue(member);

      mockGetVerifiedGitHubIdentity.mockResolvedValue({
        userId: "gh-user-456",
        username: "testuser",
      });

      mockVerifyGitHubMembership.mockResolvedValue({
        isMember: true,
        role: "admin", // GitHub upgraded to admin
      });

      const { githubOrgAccessMiddleware } = await import("./github-org-access");
      const { Hono } = await import("hono");
      const app = new Hono();

      app.use("*", async (c, next) => {
        c.set("auth" as never, { userId: "user-abc" } as never);
        await next();
      });
      app.use("/orgs/:orgId/*", githubOrgAccessMiddleware);
      app.get("/orgs/:orgId/test", (c) => {
        const orgAccess = c.get("orgAccess");
        return c.json({ role: orgAccess.role });
      });

      const res = await app.request("/orgs/org-123/test", {}, MOCK_ENV);
      const json = (await res.json()) as { role: string };

      expect(res.status).toBe(200);
      expect(json.role).toBe("member"); // DB role preserved, not GitHub

      expect(mockInsert).not.toHaveBeenCalled();
    });
  });

  describe("access gate - GitHub membership is always verified", () => {
    it("denies access when user removed from GitHub org even with Detent membership", async () => {
      const org = createOrg();
      const member = createMember({ role: "admin" }); // Has Detent membership

      mockOrgFindFirst.mockResolvedValue(org);
      mockMemberFindFirst.mockResolvedValue(member);

      mockGetVerifiedGitHubIdentity.mockResolvedValue({
        userId: "gh-user-456",
        username: "testuser",
      });

      mockVerifyGitHubMembership.mockResolvedValue({
        isMember: false, // Removed from GitHub org
        role: null,
      });

      const { githubOrgAccessMiddleware } = await import("./github-org-access");
      const { Hono } = await import("hono");
      const app = new Hono();

      app.use("*", async (c, next) => {
        c.set("auth" as never, { userId: "user-abc" } as never);
        await next();
      });
      app.use("/orgs/:orgId/*", githubOrgAccessMiddleware);
      app.get("/orgs/:orgId/test", (c) => c.json({ ok: true }));

      const res = await app.request("/orgs/org-123/test", {}, MOCK_ENV);
      const json = (await res.json()) as { error: string };

      expect(res.status).toBe(403);
      expect(json.error).toBe("Access denied");
    });
  });
});
