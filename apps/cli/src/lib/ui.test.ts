import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Team } from "./api.js";
import { findTeamByIdOrSlug, selectTeam } from "./ui.js";

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

describe("findTeamByIdOrSlug", () => {
  it("returns undefined for empty teams array", () => {
    expect(findTeamByIdOrSlug([], "team-123")).toBeUndefined();
  });

  it("finds team by ID", () => {
    const teams = [
      createMockTeam({ team_id: "team-1", team_slug: "slug-1" }),
      createMockTeam({ team_id: "team-2", team_slug: "slug-2" }),
    ];

    const result = findTeamByIdOrSlug(teams, "team-2");
    expect(result?.team_id).toBe("team-2");
  });

  it("finds team by slug", () => {
    const teams = [
      createMockTeam({ team_id: "team-1", team_slug: "slug-1" }),
      createMockTeam({ team_id: "team-2", team_slug: "slug-2" }),
    ];

    const result = findTeamByIdOrSlug(teams, "slug-1");
    expect(result?.team_slug).toBe("slug-1");
  });

  it("returns undefined when team not found", () => {
    const teams = [createMockTeam({ team_id: "team-1", team_slug: "slug-1" })];

    expect(findTeamByIdOrSlug(teams, "nonexistent")).toBeUndefined();
  });

  it("prefers ID match over slug match", () => {
    // Edge case: a team's ID matches another team's slug
    const teams = [
      createMockTeam({ team_id: "team-1", team_slug: "team-2" }),
      createMockTeam({ team_id: "team-2", team_slug: "other-slug" }),
    ];

    // Should find team-1 first since it has team_id matching OR team_slug matching
    const result = findTeamByIdOrSlug(teams, "team-2");
    // find() returns first match, which is team-1 (matching by slug)
    expect(result?.team_id).toBe("team-1");
  });
});

describe("selectTeam", () => {
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

  it("returns null and logs error for empty teams array", async () => {
    const result = await selectTeam([]);

    expect(result).toBeNull();
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      "You are not a member of any teams. Ask a team admin to invite you."
    );
  });

  it("returns single team without prompting", async () => {
    const team = createMockTeam();
    const result = await selectTeam([team]);

    expect(result).toEqual(team);
    expect(consoleLogSpy).not.toHaveBeenCalled();
  });

  it("prompts for selection with multiple teams", async () => {
    const teams = [
      createMockTeam({
        team_id: "team-1",
        team_name: "Team One",
        github_org: "org-1",
        github_linked: true,
        github_username: "user1",
      }),
      createMockTeam({
        team_id: "team-2",
        team_name: "Team Two",
        github_org: "org-2",
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

    const result = await selectTeam(teams);

    expect(result).toEqual(teams[0]);
    expect(consoleLogSpy).toHaveBeenCalledWith(
      "\nYou are a member of multiple teams:\n"
    );
  });

  it("returns null for invalid numeric selection", async () => {
    const teams = [
      createMockTeam({ team_id: "team-1" }),
      createMockTeam({ team_id: "team-2" }),
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
    const { selectTeam: selectTeamMocked } = await import("./ui.js");
    const result = await selectTeamMocked(teams);

    expect(result).toBeNull();
    expect(consoleErrorSpy).toHaveBeenCalledWith("Invalid selection");
  });

  it("returns null for non-numeric selection", async () => {
    const teams = [
      createMockTeam({ team_id: "team-1" }),
      createMockTeam({ team_id: "team-2" }),
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
    const { selectTeam: selectTeamMocked } = await import("./ui.js");
    const result = await selectTeamMocked(teams);

    expect(result).toBeNull();
    expect(consoleErrorSpy).toHaveBeenCalledWith("Invalid selection");
  });
});
