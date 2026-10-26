// Copied from github.com/brienze1/quant (frontend/src/components/MarkdownView.tsx), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
import type { ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import "./MarkdownView.css";

// Rendered view for the summary and suggestions files: GFM (tables, task lists,
// strikethrough) in the Ledger notes style.
// untrusted: text written by a session. No raw HTML, links as plain text, images as their alt text.
const untrustedComponents = {
  a: ({ children, href }: { children?: ReactNode; href?: string }) => (
    <span className="nt-md__link" title={href}>
      {children}
    </span>
  ),
  img: ({ alt }: { alt?: string }) => <span>{alt ?? ""}</span>,
};

export function MarkdownView({ content, untrusted }: { content: string; untrusted?: boolean }) {
  return (
    <div className="nt-md">
      <ReactMarkdown remarkPlugins={[remarkGfm]} {...(untrusted ? { skipHtml: true, components: untrustedComponents } : {})}>
        {content}
      </ReactMarkdown>
    </div>
  );
}
