// Helpers for rendering repository Markdown docs inside the web app. No JSX.

/** Where repository files linked from a doc are opened (GitHub blob view). */
export const DOCS_REPOSITORY_URL =
  process.env.NEXT_PUBLIC_DOCS_REPOSITORY_URL ?? "https://github.com/mhannan-dev/PayCore/blob/PayCore-Quantum/";

/** GitHub-compatible heading anchor, so "#1-start-and-sign-in" links keep working. */
export function headingSlug(text: string): string {
  return text
    .trim()
    .toLowerCase()
    // Keep letters, combining marks (Bangla vowel signs, hasanta), numbers, spaces, _ and -.
    .replace(/[^\p{L}\p{M}\p{N}\s_-]/gu, "")
    .replace(/\s/g, "-");
}

export type DocLink = { href: string; external: boolean };

/**
 * Resolves a link written relative to docs/user_guide.md.
 * In-page anchors stay local, absolute URLs open in a new tab, and relative
 * repository paths (ledger.md, ../README.md#…) open on GitHub.
 */
export function resolveDocLink(href: string): DocLink {
  if (href.startsWith("#")) return { href, external: false };
  if (/^(https?:|mailto:)/i.test(href)) return { href, external: true };
  if (/^[a-z][a-z0-9+.-]*:/i.test(href)) return { href: "#", external: false }; // javascript:, data:, …
  const [rawPath = "", hash = ""] = href.split("#", 2);
  const segments: string[] = ["docs"];
  for (const part of rawPath.split("/")) {
    if (part === "" || part === ".") continue;
    if (part === "..") segments.pop();
    else segments.push(part);
  }
  const base = DOCS_REPOSITORY_URL.endsWith("/") ? DOCS_REPOSITORY_URL : `${DOCS_REPOSITORY_URL}/`;
  return { href: `${base}${segments.join("/")}${hash ? `#${hash}` : ""}`, external: true };
}
