import type { JSX } from "react";

const CONTROL_CHARS_REGEX = /[\s\n\r\t\0]/;

/**
 * Validates that a URL is safe (http/https or relative path).
 * Rejects control characters, protocol-relative URLs, and dangerous protocols.
 */
const isValidUrl = (url: string): boolean => {
  if (CONTROL_CHARS_REGEX.test(url)) {
    return false;
  }

  if (url.startsWith("//") || url.startsWith("\\\\")) {
    return false;
  }

  if (url.startsWith("/")) {
    return true;
  }

  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
};

export function Card({
  className,
  title,
  children,
  href,
}: {
  className?: string;
  title: string;
  children: React.ReactNode;
  href: string;
}): JSX.Element {
  const safeHref = isValidUrl(href)
    ? `${href}?utm_source=create-turbo&utm_medium=basic&utm_campaign=create-turbo`
    : "#";

  return (
    <a
      className={className}
      href={safeHref}
      rel="noopener noreferrer"
      target="_blank"
    >
      <h2>
        {title} <span>-&gt;</span>
      </h2>
      <p>{children}</p>
    </a>
  );
}
