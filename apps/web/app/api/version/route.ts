import { list } from "@vercel/blob";
import { NextResponse } from "next/server";

const MANIFEST_PATH = "releases/manifest.json";
const VERSION_PREFIX = /^v/;

export const GET = async (): Promise<NextResponse> => {
  const { blobs } = await list({ prefix: MANIFEST_PATH });

  if (!blobs[0]) {
    return NextResponse.json(
      { error: "No releases available" },
      { status: 404 }
    );
  }

  const res = await fetch(blobs[0].url);
  const manifest = (await res.json()) as {
    latest?: string;
    versions?: string[];
  };

  if (!manifest.latest) {
    return NextResponse.json(
      { error: "No releases available" },
      { status: 404 }
    );
  }

  return NextResponse.json({
    version: manifest.latest.replace(VERSION_PREFIX, ""),
    latest: manifest.latest,
  });
};
