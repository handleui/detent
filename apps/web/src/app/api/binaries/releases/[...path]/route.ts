import { list } from "@vercel/blob";
import { type NextRequest, NextResponse } from "next/server";

/**
 * Proxies requests to Vercel Blob storage for CLI binary distribution.
 * This allows a stable URL (detent.sh/api/binaries/...) regardless of blob store ID.
 */
export const GET = async (
  _request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
): Promise<NextResponse> => {
  const { path } = await params;
  const blobPath = `releases/${path.join("/")}`;

  try {
    // List blobs with the exact path prefix
    const { blobs } = await list({ prefix: blobPath, limit: 1 });

    if (blobs.length === 0) {
      return NextResponse.json({ error: "Not found" }, { status: 404 });
    }

    const blob = blobs[0];

    // For manifest.json, fetch and return the content
    if (blobPath.endsWith(".json")) {
      const response = await fetch(blob.url);
      const data = await response.json();
      return NextResponse.json(data, {
        headers: {
          "Cache-Control": "public, max-age=60, s-maxage=60",
        },
      });
    }

    // For binary files, redirect to the blob URL
    return NextResponse.redirect(blob.url, 302);
  } catch (error) {
    console.error("Blob proxy error:", error);
    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 }
    );
  }
};
