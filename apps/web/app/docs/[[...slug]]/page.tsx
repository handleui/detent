import type { TOCItemType } from "fumadocs-core/toc";
import { DocsBody, DocsPage } from "fumadocs-ui/page";
import { notFound } from "next/navigation";
import type { ComponentType, ReactElement } from "react";
import { source } from "@/lib/source";

interface PageProps {
  params: Promise<{ slug?: string[] }>;
}

interface DocPageData {
  body: ComponentType;
  toc: TOCItemType[];
}

export default async function Page({
  params,
}: PageProps): Promise<ReactElement> {
  const { slug } = await params;
  const page = source.getPage(slug);
  if (!page) notFound();

  const { body: MDX, toc } = page.data as DocPageData;

  return (
    <DocsPage toc={toc}>
      <DocsBody>
        <MDX />
      </DocsBody>
    </DocsPage>
  );
}

export function generateStaticParams() {
  return source.generateParams();
}
