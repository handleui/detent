import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StatusResponse, Team } from "../../lib/api.js";

// Mock external dependencies
vi.mock("@detent/git", () => ({
  findGitRoot: vi.fn(),
}));

vi.mock("../../lib/auth.js", () => ({
  getAccessToken: vi.fn(),
}));

vi.mock("../../lib/api.js", () => ({
  getTeams: vi.fn(),
  getLinkStatus: vi.fn(),
  getAuthorizeUrl: vi.fn(),
  submitCallback: vi.fn(),
  unlinkGithub: vi.fn(),
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

  describe("github link command", () => {
    it("exits if not in git repository", async () => {
      const { findGitRoot } = await import("@detent/git");
      vi.mocked(findGitRoot).mockResolvedValue(null);

      const { githubLinkCommand } = await import("./github.js");

      await expect(githubLinkCommand.run?.({ args: {} })).rejects.toThrow(
        ExitError
      );
      expect(consoleErrorSpy).toHaveBeenCalledWith("Not in a git repository.");
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("exits if not logged in", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockRejectedValue(new Error("Not logged in"));

      const { githubLinkCommand } = await import("./github.js");

      await expect(githubLinkCommand.run?.({ args: {} })).rejects.toThrow(
        ExitError
      );
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "Not logged in. Run `detent auth login` first."
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("exits if team not found when --team provided", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { findTeamByIdOrSlug } = await import("../../lib/ui.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [createMockTeam()] });
      vi.mocked(findTeamByIdOrSlug).mockReturnValue(undefined);

      const { githubLinkCommand } = await import("./github.js");

      await expect(
        githubLinkCommand.run?.({ args: { team: "nonexistent" } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "Team not found: nonexistent"
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("shows already linked message if team is already linked", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { findTeamByIdOrSlug } = await import("../../lib/ui.js");

      const linkedTeam = createMockTeam({
        github_linked: true,
        github_username: "linked-user",
      });

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [linkedTeam] });
      vi.mocked(findTeamByIdOrSlug).mockReturnValue(linkedTeam);

      const { githubLinkCommand } = await import("./github.js");
      await githubLinkCommand.run?.({ args: { team: "test-team" } });

      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nAlready linked to GitHub as @linked-user"
      );
      expect(processExitSpy).not.toHaveBeenCalled();
    });

    it("exits if selectTeam returns null", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { selectTeam } = await import("../../lib/ui.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({
        teams: [createMockTeam(), createMockTeam({ team_id: "team-2" })],
      });
      vi.mocked(selectTeam).mockResolvedValue(null);

      const { githubLinkCommand } = await import("./github.js");

      await expect(githubLinkCommand.run?.({ args: {} })).rejects.toThrow(
        ExitError
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });
  });

  describe("status command", () => {
    it("exits if not in git repository", async () => {
      const { findGitRoot } = await import("@detent/git");
      vi.mocked(findGitRoot).mockResolvedValue(null);

      const { statusCommand } = await import("./status.js");

      await expect(
        statusCommand.run?.({ args: { all: false } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith("Not in a git repository.");
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("exits if not logged in", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockRejectedValue(new Error("Not logged in"));

      const { statusCommand } = await import("./status.js");

      await expect(
        statusCommand.run?.({ args: { all: false } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "Not logged in. Run `detent auth login` first."
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("displays all teams when --all flag provided", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");

      const teams = [
        createMockTeam({
          team_id: "team-1",
          team_name: "Team One",
          github_linked: true,
          github_username: "user1",
        }),
        createMockTeam({
          team_id: "team-2",
          team_name: "Team Two",
          github_linked: false,
        }),
      ];

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams });

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: { all: true } });

      expect(consoleLogSpy).toHaveBeenCalledWith("\nGitHub Link Status\n");
      expect(consoleLogSpy).toHaveBeenCalledWith("\nTeam: Team One");
      expect(consoleLogSpy).toHaveBeenCalledWith("\nTeam: Team Two");
    });

    it("displays single team status", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams, getLinkStatus } = await import("../../lib/api.js");
      const { findTeamByIdOrSlug } = await import("../../lib/ui.js");

      const team = createMockTeam({
        team_name: "My Team",
        github_linked: true,
        github_username: "myuser",
      });

      const statusResponse: StatusResponse = {
        team_id: "team-123",
        team_name: "My Team",
        team_slug: "my-team",
        github_org: "my-org",
        github_linked: true,
        github_user_id: "12345",
        github_username: "myuser",
        github_linked_at: "2024-01-01T00:00:00Z",
      };

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [team] });
      vi.mocked(findTeamByIdOrSlug).mockReturnValue(team);
      vi.mocked(getLinkStatus).mockResolvedValue(statusResponse);

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: { team: "my-team", all: false } });

      expect(consoleLogSpy).toHaveBeenCalledWith("\nGitHub Link Status\n");
      expect(consoleLogSpy).toHaveBeenCalledWith("Team:        My Team");
      expect(consoleLogSpy).toHaveBeenCalledWith("GitHub:      @myuser");
    });

    it("exits if team not found", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams } = await import("../../lib/api.js");
      const { findTeamByIdOrSlug } = await import("../../lib/ui.js");

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [createMockTeam()] });
      vi.mocked(findTeamByIdOrSlug).mockReturnValue(undefined);

      const { statusCommand } = await import("./status.js");

      await expect(
        statusCommand.run?.({ args: { team: "nonexistent", all: false } })
      ).rejects.toThrow(ExitError);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        "Team not found: nonexistent"
      );
      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it("displays not linked status for unlinked team", async () => {
      const { findGitRoot } = await import("@detent/git");
      const { getAccessToken } = await import("../../lib/auth.js");
      const { getTeams, getLinkStatus } = await import("../../lib/api.js");
      const { selectTeam } = await import("../../lib/ui.js");

      const team = createMockTeam({ github_linked: false });

      const statusResponse: StatusResponse = {
        team_id: "team-123",
        team_name: "Test Team",
        team_slug: "test-team",
        github_org: "test-org",
        github_linked: false,
        github_user_id: null,
        github_username: null,
        github_linked_at: null,
      };

      vi.mocked(findGitRoot).mockResolvedValue("/repo");
      vi.mocked(getAccessToken).mockResolvedValue("token-123");
      vi.mocked(getTeams).mockResolvedValue({ teams: [team] });
      vi.mocked(selectTeam).mockResolvedValue(team);
      vi.mocked(getLinkStatus).mockResolvedValue(statusResponse);

      const { statusCommand } = await import("./status.js");
      await statusCommand.run?.({ args: { all: false } });

      expect(consoleLogSpy).toHaveBeenCalledWith("\nStatus: ❌ Not linked");
      expect(consoleLogSpy).toHaveBeenCalledWith(
        "\nRun `detent link` to connect your GitHub account."
      );
    });
  });

  describe("link command (index)", () => {
    it("has correct meta information", async () => {
      const { linkCommand } = await import("./index.js");

      expect(linkCommand.meta?.name).toBe("link");
      expect(linkCommand.meta?.description).toBe(
        "Link your GitHub account to your Detent team"
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
});
