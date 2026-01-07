import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ProjectConfig } from "../../lib/config.js";

// Mock external dependencies
vi.mock("@detent/git", () => ({
  findGitRoot: vi.fn(),
}));

vi.mock("../../lib/auth.js", () => ({
  getAccessToken: vi.fn(),
}));

vi.mock("../../lib/api.js", () => ({
  getOrganizations: vi.fn(),
}));

vi.mock("../../lib/config.js", () => ({
  getProjectConfig: vi.fn(),
  saveProjectConfig: vi.fn(),
  removeProjectConfig: vi.fn(),
}));

vi.mock("../../lib/ui.js", () => ({
  findOrganizationByIdOrSlug: vi.fn(),
  selectOrganization: vi.fn(),
}));

interface Organization {
  organization_id: string;
  organization_name: string;
  organization_slug: string;
  github_org: string;
  role: string;
  github_linked: boolean;
  github_username: string | null;
}

const createMockOrganization = (
  overrides: Partial<Organization> = {}
): Organization => ({
  organization_id: "org-123",
  organization_name: "Test Organization",
  organization_slug: "test-org",
  github_org: "test-org",
  role: "member",
  github_linked: false,
  github_username: null,
  ...overrides,
});

// Custom error to simulate process.exit
class ExitError extends Error {
  code: number;
  constructor(code: number) {
    super(`process.exit(${code})`);
    this.code = code;
  }
}

describe("link commands", () => {
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;
  let consoleLogSpy: ReturnType<typeof vi.spyOn>;
  let processExitSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    consoleLogSpy = vi
      .spyOn(console, "log")
      .mockImplementation(() => undefined);
    processExitSpy = vi.spyOn(process, "exit").mockImplementation((code) => {
      throw new ExitError(code as number);
    });
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    consoleLogSpy.mockRestore();
    processExitSpy.mockRestore();
    vi.clearAllMocks();
  });

  describe("link command (index)", () => {
    it("exits if not in git repository", async () => {
      const { findGitRoot } = await import("@detent/git");
      vi.mocked(findGitRoot).mockResolvedValue(null);

      const { linkCommand } = await import("./index.js");

      await expect(
        linkCommand.run?.({ args: { force: false } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith("Not in a git repository.");
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("exits if not logged in", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getProjectConfig } = await import("../../lib/config.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(null);
      vi.mocked(getAccessToken).mockRejectedValue(new Error("Not logged in"));

      const { linkCommand } = await import("./index.js");

      await expect(
        linkCommand.run?.({ args: { force: false } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "Not logged in. Run `detent auth login` first."
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("shows already linked message if repo is linked without force flag", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getProjectConfig } = await import("../../lib/config.js");

      const existingConfig: ProjectConfig = {
        organizationId: "org-123",
        organizationSlug: "test-org",
      };

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getProjectConfig).mockReturnValue(existingConfig);

      const { linkCommand } = await import("./index.js");
      await linkCommand.run?.({ args: { force: false } });

      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nThis repository is already linked to organization: test-org"
      );
      expect(processExitSpy).not.toHaveBeenCalled();
    });

    it("allows relinking with --force flag", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getOrganizations } = await import("../../lib/api.js");
      const { getProjectConfig, saveProjectConfig } = await import(
        "../../lib/config.js"
      );
      const { findOrganizationByIdOrSlug } = await import("../../lib/ui.js");

      const existingConfig: ProjectConfig = {
        organizationId: "org-old",
        organizationSlug: "old-org",
      };

      const newOrganization = createMockOrganization({
        organization_id: "org-new",
        organization_slug: "new-org",
        organization_name: "New Organization",
      });

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(existingConfig);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getOrganizations).mockResolvedValue({
        organizations: [newOrganization],
      });
      vi.mocked(findOrganizationByIdOrSlug).mockReturnValue(newOrganization);

      const { linkCommand } = await import("./index.js");
      await linkCommand.run?.({
        args: { force: true, organization: "new-org" },
      });

      expect(saveProjectConfig).toHaveBeenCalledWith("/repo", {
        organizationId: "org-new",
        organizationSlug: "new-org",
      });
    });

    it("exits if organization not found when --organization provided", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getOrganizations } = await import("../../lib/api.js");
      const { getProjectConfig } = await import("../../lib/config.js");
      const { findOrganizationByIdOrSlug } = await import("../../lib/ui.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(null);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getOrganizations).mockResolvedValue({
        organizations: [createMockOrganization()],
      });
      vi.mocked(findOrganizationByIdOrSlug).mockReturnValue(undefined);

      const { linkCommand } = await import("./index.js");

      await expect(
        linkCommand.run?.({
          args: { organization: "nonexistent", force: false },
        })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "Organization not found: nonexistent"
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("has correct meta information", async () => {
      const { linkCommand } = await import("./index.js");

      expect(linkCommand.meta?.name).toBe("link");
      expect(linkCommand.meta?.description).toBe(
        "Link this repository to a Detent organization"
      );
    });

    it("has status subcommand", async () => {
      const { linkCommand } = await import("./index.js");

      expect(linkCommand.subCommands).toBeDefined();
      expect(linkCommand.subCommands?.status).toBeDefined();
    });

    it("has unlink subcommand", async () => {
      const { linkCommand } = await import("./index.js");

      expect(linkCommand.subCommands).toBeDefined();
      expect(linkCommand.subCommands?.unlink).toBeDefined();
    });
  });

  describe("status command", () => {
    it("exits if not in git repository", async () => {
      const { findGitRoot } = await import("@detent/git");
      vi.mocked(findGitRoot).mockResolvedValue(null);

      const { statusCommand } = await import("./status.js");

      await expect(statusCommand.run?.({ args: {} })).rejects.toThrow(
        ExitError
      );
      expect(consoleErrorSpy).toHaveBeenCalledWith("Not in a git repository.");
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("shows not linked message if repo is not linked", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getProjectConfig } = await import("../../lib/config.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(null);

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: {} });

      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nThis repository is not linked to any organization."
      );
    });

    it("shows link status when repo is linked", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getOrganizations } = await import("../../lib/api.js");
      const { getProjectConfig } = await import("../../lib/config.js");

      const projectConfig: ProjectConfig = {
        organizationId: "org-123",
        organizationSlug: "test-org",
      };

      const organization = createMockOrganization({
        organization_id: "org-123",
        organization_name: "Test Organization",
        github_linked: true,
        github_username: "testuser",
      });

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(projectConfig);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getOrganizations).mockResolvedValue({
        organizations: [organization],
      });

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: {} });

      expect(consoleLogSpy).toHaveBeenCalledWith("\nLink Status\n");
      expect(consoleLogSpy).toHaveBeenCalledWith(
        "Organization ID:     org-123"
      );
      expect(consoleLogSpy).toHaveBeenCalledWith(
        "Organization Slug:   test-org"
      );
    });

    it("shows warning if not member of linked organization", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getOrganizations } = await import("../../lib/api.js");
      const { getProjectConfig } = await import("../../lib/config.js");

      const projectConfig: ProjectConfig = {
        organizationId: "org-other",
        organizationSlug: "other-org",
      };

      const organization = createMockOrganization({
        organization_id: "org-123",
      });

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(projectConfig);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getOrganizations).mockResolvedValue({
        organizations: [organization],
      });

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: {} });

      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nWarning: You are not a member of the linked organization."
      );
    });
  });

  describe("unlink command", () => {
    it("exits if not in git repository", async () => {
      const { findGitRoot } = await import("@detent/git");
      vi.mocked(findGitRoot).mockResolvedValue(null);

      const { unlinkCommand } = await import("./unlink.js");

      await expect(
        unlinkCommand.run?.({ args: { force: false } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith("Not in a git repository.");
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("shows not linked message if repo is not linked", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getProjectConfig } = await import("../../lib/config.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(null);

      const { unlinkCommand } = await import("./unlink.js");
      await unlinkCommand.run?.({ args: { force: false } });

      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nThis repository is not linked to any organization."
      );
    });

    it("removes project config with --force flag", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getProjectConfig, removeProjectConfig } = await import(
        "../../lib/config.js"
      );

      const projectConfig: ProjectConfig = {
        organizationId: "org-123",
        organizationSlug: "test-org",
      };

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(projectConfig);

      const { unlinkCommand } = await import("./unlink.js");
      await unlinkCommand.run?.({ args: { force: true } });

      expect(removeProjectConfig).toHaveBeenCalledWith("/repo");
      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nSuccessfully unlinked repository from organization."
      );
    });
  });
});
