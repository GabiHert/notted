import type { ReviewPhase } from "../review";
import { MarkdownView } from "./MarkdownView";
import "./SummaryCard.css";

type Props = { suggestionsMd: string; phase: ReviewPhase; message: string; sessionOpen: boolean; onReview: () => void };

export function SuggestionsCard({ suggestionsMd, phase, message, sessionOpen, onReview }: Props) {
  const running = phase === "running";
  return (
    <section className="nt-card">
      <div className="nt-card__header">Suggestions</div>
      {suggestionsMd ? (
        <>
          <div className="nt-review">
            <button className="nt-btn nt-btn--primary" disabled={sessionOpen || running} onClick={onReview}>
              Review suggestions
            </button>
            {running ? (
              <p className="nt-session__note">Reviewing suggestions… When Claude Code needs you, a form appears at the top of this page.</p>
            ) : sessionOpen ? (
              <p className="nt-session__note">Close the Claude Code session below first. The review and the session use the same conversation.</p>
            ) : phase === "done" ? (
              <>
                <p className="nt-session__note">Review finished.</p>
                {message !== "" && (
                  <div className="nt-review__reply">
                    <MarkdownView content={message} untrusted />
                  </div>
                )}
              </>
            ) : phase === "error" ? (
              <div className="nt-card nt-card--error" role="alert">
                {message}
              </div>
            ) : (
              <p className="nt-session__note">Claude Code shows the suggestions as a form at the top of this page. Nothing changes until you answer.</p>
            )}
          </div>
          <MarkdownView content={suggestionsMd} />
        </>
      ) : (
        <p className="nt-summary__note nt-summary__note--bare">No suggestions yet.</p>
      )}
    </section>
  );
}
