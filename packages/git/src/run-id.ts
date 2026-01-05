import { createHash } from "node:crypto";
import { tmpdir } from "node:os";
import { join, normalize, resolve, sep } from "node:path";
import { getCurrentRefs } from "./operations.js";
import type { CommitSHA, RunID, RunIDInfo, TreeHash } from "./types.js";
import { isValidRunID } from "./utils.js";

const RUNID_LENGTH = 16;
const WORKTREE_PREFIX = "detent-" as const;

export const computeRunID = (
  treeHash: TreeHash,
  commitSHA: CommitSHA
): RunID => {
  const h = createHash("sha256");
  h.update(treeHash + commitSHA);
  return h.digest("hex").substring(0, RUNID_LENGTH) as RunID;
};

export const computeCurrentRunID = async (
  repoRoot: string
): Promise<RunIDInfo> => {
  const refs = await getCurrentRefs(repoRoot);
  const runID = computeRunID(refs.treeHash, refs.commitSHA);
  return {
    runID,
    treeHash: refs.treeHash,
    commitSHA: refs.commitSHA,
  };
};

export const createEphemeralWorktreePath = (runID: RunID): string => {
  if (!isValidRunID(runID)) {
    throw new Error("invalid run ID: must be a hex string");
  }

  const tempDir = tmpdir();
  const proposedPath = join(tempDir, `${WORKTREE_PREFIX}${runID}`);

  const normalizedTemp = normalize(resolve(tempDir));
  const normalizedProposed = normalize(resolve(proposedPath));

  if (
    !normalizedProposed.startsWith(normalizedTemp + sep) &&
    normalizedProposed !== normalizedTemp
  ) {
    throw new Error("path traversal detected in run ID");
  }

  const pathComponents = normalizedProposed.split(sep);
  const tempComponents = normalizedTemp.split(sep);

  if (pathComponents.length !== tempComponents.length + 1) {
    throw new Error("path traversal detected: invalid path depth");
  }

  return proposedPath;
};
