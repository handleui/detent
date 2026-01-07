import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Team } from "../../lib/api.js";
import type { ProjectConfig } from "../../lib/config.js";

// Mock external dependencies
vi.mock("@detent/git", () => ({
  findGitRoot: vi.fn(),
}));

vi.mock("../../lib/auth.js", () => ({
  getAccessToken: vi.fn(),
}));

vi.mock("../../lib/api.js", () => ({
  getTeams: vi.fn(),
}));

vi.mock("../../lib/config.js", () => ({
  getProjectConfig: vi.fn(),
  saveProjectConfig: vi.fn(),
  removeProjectConfig: vi.fn(),
}));

vi.mock("../../lib/ui.js", () => ({
  findTeamByIdOrSlug: vi.fn(),
  selectTeam: vi.fn(),
}));

const createMockTeam = (overrides: Partial<Team> = {}): Team => ({
  team_id: "team-123",
  team_name: "Test Team",
  team_slug: "test-team",
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
        teamId: "team-123",
        teamSlug: "test-team",
      };

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getProjectConfig).mockReturnValue(existingConfig);

      const { linkCommand } = await import("./index.js");
      await linkCommand.run?.({ args: { force: false } });

      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nThis repository is already linked to team: test-team"
      );
      expect(processExitSpy).not.toHaveBeenCalled();
    });

    it("allows relinking with --force flag", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { getProjectConfig, saveProjectConfig } = await import(
        "../../lib/config.js"
      );
      const { findTeamByIdOrSlug } = await import("../../lib/ui.js");

      const existingConfig: ProjectConfig = {
        teamId: "team-old",
        teamSlug: "old-team",
      };

      const newTeam = createMockTeam({
        team_id: "team-new",
        team_slug: "new-team",
        team_name: "New Team",
      });

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(existingConfig);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [newTeam] });
      vi.mocked(findTeamByIdOrSlug).mockReturnValue(newTeam);

      const { linkCommand } = await import("./index.js");
      await linkCommand.run?.({ args: { force: true, team: "new-team" } });

      expect(saveProjectConfig).toHaveBeenCalledWith("/repo", {
        teamId: "team-new",
        teamSlug: "new-team",
      });
    });

    it("exits if team not found when --team provided", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { getProjectConfig } = await import("../../lib/config.js");
      const { findTeamByIdOrSlug } = await import("../../lib/ui.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(null);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [createMockTeam()] });
      vi.mocked(findTeamByIdOrSlug).mockReturnValue(undefined);

      const { linkCommand } = await import("./index.js");

      await expect(
        linkCommand.run?.({ args: { team: "nonexistent", force: false } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "Team not found: nonexistent"
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("has correct meta information", async () => {
      const { linkCommand } = await import("./index.js");

      expect(linkCommand.meta?.name).toBe("link");
      expect(linkCommand.meta?.description).toBe(
        "Link this repository to a Detent team"
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
        "\nThis repository is not linked to any team."
      );
    });

    it("shows link status when repo is linked", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { getProjectConfig } = await import("../../lib/config.js");

      const projectConfig: ProjectConfig = {
        teamId: "team-123",
        teamSlug: "test-team",
      };

      const team = createMockTeam({
        team_id: "team-123",
        team_name: "Test Team",
        github_linked: true,
        github_username: "testuser",
      });

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(projectConfig);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [team] });

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: {} });

      expect(consoleLogSpy).toHaveBeenCalledWith("\nLink Status\n");
      expect(consoleLogSpy).toHaveBeenCalledWith("Team ID:     team-123");
      expect(consoleLogSpy).toHaveBeenCalledWith("Team Slug:   test-team");
    });

    it("shows warning if not member of linked team", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { getProjectConfig } = await import("../../lib/config.js");

      const projectConfig: ProjectConfig = {
        teamId: "team-other",
        teamSlug: "other-team",
      };

      const team = createMockTeam({ team_id: "team-123" });

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(projectConfig);
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [team] });

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: {} });

      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nWarning: You are not a member of the linked team."
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
        "\nThis repository is not linked to any team."
      );
    });

    it("removes project config with --force flag", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getProjectConfig, removeProjectConfig } = await import(
        "../../lib/config.js"
      );

      const projectConfig: ProjectConfig = {
        teamId: "team-123",
        teamSlug: "test-team",
      };

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getProjectConfig).mockReturnValue(projectConfig);

      const { unlinkCommand } = await import("./unlink.js");
      await unlinkCommand.run?.({ args: { force: true } });

      expect(removeProjectConfig).toHaveBeenCalledWith("/repo");
      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nSuccessfully unlinked repository from team."
      );
    });
  });
});
