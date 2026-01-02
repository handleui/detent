import { list } from "@vercel/blob";
import { NextResponse } from "next/server";

const MANIFEST_PATH = "releases/manifest.json";

interface RouteParams {
  params: Promise<{ path: string[] }>;
}

const getLatestVersion = async (): Promise<string | null> => {
  const { blobs } = await list({ prefix: MANIFEST_PATH });
  if (!blobs[0]) {
    return null;
  }

  const res = await fetch(blobs[0].url);
  const manifest = (await res.json()) as { latest?: string };
  return manifest.latest || null;
};

export const GET = async (
  _request: Request,
  { params }: RouteParams
): Promise<NextResponse> => {
  const { path } = await params;

  // Handle manifest request: /api/binaries/releases/manifest.json
  if (
    path.length === 2 &&
    path[0] === "releases" &&
    path[1] === "manifest.json"
  ) {
    const { blobs } = await list({ prefix: MANIFEST_PATH });
    if (!blobs[0]) {
      return NextResponse.json({ error: "No manifest found" }, { status: 404 });
    }
    return NextResponse.redirect(blobs[0].url);
  }

  if (path.length !== 2) {
    return NextResponse.json(
      { error: "Expected: /api/binaries/{version}/{filename}" },
      { status: 400 }
    );
  }

  const [versionInput, filename] = path as [string, string];

  // Resolve "latest" to actual version
  let version = versionInput;
  if (version === "latest") {
    const latest = await getLatestVersion();
    if (!latest) {
      return NextResponse.json(
        { error: "No releases available" },
        { status: 404 }
      );
    }
    version = latest;
  }

  // Ensure v prefix
  if (!version.startsWith("v")) {
    version = `v${version}`;
  }

  // Find blob
  const blobPath = `releases/${version}/${filename}`;
  const { blobs } = await list({ prefix: blobPath });

  if (!blobs[0]) {
    return NextResponse.json(
      { error: `Not found: ${version}/${filename}` },
      { status: 404 }
    );
  }

  return NextResponse.redirect(blobs[0].url);
};
