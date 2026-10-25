// Copied from github.com/brienze1/quant (frontend/src/components/MarkdownView.tsx), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import "./MarkdownView.css";

// Rendered view for the summary and suggestions files: GFM (tables, task lists,
// strikethrough) in the Ledger notes style.
export function MarkdownView({ content }: { content: string }) {
  return (
    <div className="nt-md">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
    </div>
  );
}
