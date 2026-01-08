import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Organization } from "./api.js";
import { findOrganizationByIdOrSlug, selectOrganization } from "./ui.js";

const createMockOrganization = (
  overrides: Partial<Organization> = {}
): Organization => ({
  organization_id: "org-123",
  organization_name: "Test Organization",
  organization_slug: "test-org",
  github_org: "test-github-org",
  role: "member",
  github_linked: false,
  github_username: null,
  ...overrides,
});

describe("findOrganizationByIdOrSlug", () => {
  it("returns undefined for empty organizations array", () => {
    expect(findOrganizationByIdOrSlug([], "org-123")).toBeUndefined();
  });

  it("finds organization by ID", () => {
    const organizations = [
      createMockOrganization({
        organization_id: "org-1",
        organization_slug: "slug-1",
      }),
      createMockOrganization({
        organization_id: "org-2",
        organization_slug: "slug-2",
      }),
    ];

    const result = findOrganizationByIdOrSlug(organizations, "org-2");
    expect(result?.organization_id).toBe("org-2");
  });

  it("finds organization by slug", () => {
    const organizations = [
      createMockOrganization({
        organization_id: "org-1",
        organization_slug: "slug-1",
      }),
      createMockOrganization({
        organization_id: "org-2",
        organization_slug: "slug-2",
      }),
    ];

    const result = findOrganizationByIdOrSlug(organizations, "slug-1");
    expect(result?.organization_slug).toBe("slug-1");
  });

  it("returns undefined when organization not found", () => {
    const organizations = [
      createMockOrganization({
        organization_id: "org-1",
        organization_slug: "slug-1",
      }),
    ];

    expect(
      findOrganizationByIdOrSlug(organizations, "nonexistent")
    ).toBeUndefined();
  });

  it("prefers ID match over slug match", () => {
    // Edge case: an organization's ID matches another organization's slug
    const organizations = [
      createMockOrganization({
        organization_id: "org-1",
        organization_slug: "org-2",
      }),
      createMockOrganization({
        organization_id: "org-2",
        organization_slug: "other-slug",
      }),
    ];

    // Should find org-1 first since it has organization_id matching OR organization_slug matching
    const result = findOrganizationByIdOrSlug(organizations, "org-2");
    // find() returns first match, which is org-1 (matching by slug)
    expect(result?.organization_id).toBe("org-1");
  });
});

describe("selectOrganization", () => {
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;
  let consoleLogSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    consoleLogSpy = vi
      .spyOn(console, "log")
      .mockImplementation(() => undefined);
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    consoleLogSpy.mockRestore();
    vi.restoreAllMocks();
  });

  it("returns null and logs error for empty organizations array", async () => {
    const result = await selectOrganization([]);

    expect(result).toBeNull();
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      "You are not a member of any organizations. You must be a member of a GitHub organization where Detent is installed."
    );
  });

  it("returns single organization without prompting", async () => {
    const organization = createMockOrganization();
    const result = await selectOrganization([organization]);

    expect(result).toEqual(organization);
    expect(consoleLogSpy).not.toHaveBeenCalled();
  });

  it("prompts for selection with multiple organizations", async () => {
    const organizations = [
      createMockOrganization({
        organization_id: "org-1",
        organization_name: "Organization One",
        github_org: "github-org-1",
        github_linked: true,
        github_username: "user1",
      }),
      createMockOrganization({
        organization_id: "org-2",
        organization_name: "Organization Two",
        github_org: "github-org-2",
        github_linked: false,
      }),
    ];

    // Mock readline
    vi.mock("node:readline", () => ({
      createInterface: () => ({
        question: (_prompt: string, callback: (answer: string) => void) => {
          callback("1");
        },
        close: vi.fn(),
      }),
    }));

    const result = await selectOrganization(organizations);

    expect(result).toEqual(organizations[0]);
    expect(consoleLogSpy).toHaveBeenCalledWith(
      "\nYou are a member of multiple organizations:\n"
    );
  });

  it("returns null for invalid numeric selection", async () => {
    const organizations = [
      createMockOrganization({ organization_id: "org-1" }),
      createMockOrganization({ organization_id: "org-2" }),
    ];

    // Mock readline with invalid selection
    vi.doMock("node:readline", () => ({
      createInterface: () => ({
        question: (_prompt: string, callback: (answer: string) => void) => {
          callback("99");
        },
        close: vi.fn(),
      }),
    }));

    // Re-import to pick up the mock
    const { selectOrganization: selectOrganizationMocked } = await import(
      "./ui.js"
    );
    const result = await selectOrganizationMocked(organizations);

    expect(result).toBeNull();
    expect(consoleErrorSpy).toHaveBeenCalledWith("Invalid selection");
  });

  it("returns null for non-numeric selection", async () => {
    const organizations = [
      createMockOrganization({ organization_id: "org-1" }),
      createMockOrganization({ organization_id: "org-2" }),
    ];

    // Mock readline with non-numeric input
    vi.doMock("node:readline", () => ({
      createInterface: () => ({
        question: (_prompt: string, callback: (answer: string) => void) => {
          callback("abc");
        },
        close: vi.fn(),
      }),
    }));

    // Re-import to pick up the mock
    const { selectOrganization: selectOrganizationMocked } = await import(
      "./ui.js"
    );
    const result = await selectOrganizationMocked(organizations);

    expect(result).toBeNull();
    expect(consoleErrorSpy).toHaveBeenCalledWith("Invalid selection");
  });
});
