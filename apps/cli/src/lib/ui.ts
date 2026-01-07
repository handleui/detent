/**
 * Shared UI utilities for the Detent CLI
 */

import type { Team } from "./api.js";

/**
 * Find a team by ID or slug
 *
 * Returns the matching team or undefined if not found.
 */
export const findTeamByIdOrSlug = (
  teams: Team[],
  idOrSlug: string
): Team | undefined =>
  teams.find((t) => t.team_id === idOrSlug || t.team_slug === idOrSlug);

/**
 * Prompt user to select a team from a list
 *
 * Returns the selected team or null if no valid selection was made.
 */
export const selectTeam = async (teams: Team[]): Promise<Team | null> => {
  if (teams.length === 0) {
    console.error(
      "You are not a member of any teams. Ask a team admin to invite you."
    );
    return null;
  }

  const firstTeam = teams[0];
  if (teams.length === 1 && firstTeam) {
    return firstTeam;
  }

  // Multiple teams - let user select
  console.log("\nYou are a member of multiple teams:\n");
  for (const [i, team] of teams.entries()) {
    const linked = team.github_linked
      ? `(linked: @${team.github_username})`
      : "(not linked)";
    console.log(`  ${i + 1}. ${team.team_name} (${team.github_org}) ${linked}`);
  }

  const readline = await import("node:readline");
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  const answer = await new Promise<string>((resolve) => {
    rl.question("\nSelect team number: ", resolve);
  });
  rl.close();

  const index = Number.parseInt(answer, 10) - 1;
  if (Number.isNaN(index) || index < 0 || index >= teams.length) {
    console.error("Invalid selection");
    return null;
  }

  const selectedTeam = teams[index];
  return selectedTeam ?? null;
};
