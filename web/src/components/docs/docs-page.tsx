import { Children, isValidElement, type ReactNode } from "react";
import Link from "next/link";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { headingSlug, resolveDocLink } from "@/lib/docs";
import "./docs.css";

/** Plain text of rendered heading children, for GitHub-compatible anchors. */
function textOf(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (isValidElement<{ children?: ReactNode }>(node)) return textOf(node.props.children);
  return "";
}

const heading = (Tag: "h2" | "h3" | "h4") =>
  function Heading({ children }: { children?: ReactNode }) {
    return <Tag id={headingSlug(textOf(Children.toArray(children)))}>{children}</Tag>;
  };

const components: Components = {
  h2: heading("h2"),
  h3: heading("h3"),
  h4: heading("h4"),
  a({ href, children }) {
    const link = resolveDocLink(href ?? "#");
    return link.external
      ? <a href={link.href} target="_blank" rel="noopener noreferrer">{children}</a>
      : <a href={link.href}>{children}</a>;
  },
  table({ children }) {
    return <div className="docs-table-wrap"><table>{children}</table></div>;
  },
};

/** Renders a repository Markdown document. Raw HTML in the source is not rendered. */
export function DocsPage({ markdown }: { markdown: string }) {
  return (
    <div className="docs-page">
      <nav className="docs-topnav" aria-label="Documentation">
        <Link href="/">← PayCore console</Link>
        <Link href="/login">Sign in</Link>
      </nav>
      <article className="docs-article">
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>{markdown}</ReactMarkdown>
      </article>
    </div>
  );
}
