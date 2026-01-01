import { DocsLayout } from "fumadocs-ui/layouts/docs";
import type { ReactElement, ReactNode } from "react";
import { source } from "@/lib/source";

export default function Layout({
  children,
}: {
  children: ReactNode;
}): ReactElement {
  return <DocsLayout tree={source.pageTree}>{children}</DocsLayout>;
}
