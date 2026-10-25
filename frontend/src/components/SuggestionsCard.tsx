import { MarkdownView } from "./MarkdownView";
import "./SummaryCard.css";

export function SuggestionsCard({ suggestionsMd }: { suggestionsMd: string }) {
  return (
    <section className="nt-card">
      <div className="nt-card__header">Suggestions</div>
      {suggestionsMd ? <MarkdownView content={suggestionsMd} /> : <p className="nt-summary__note nt-summary__note--bare">No suggestions yet.</p>}
    </section>
  );
}
