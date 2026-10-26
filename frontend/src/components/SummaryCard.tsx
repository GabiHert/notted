import { GenerateSummary, type wails } from "../api/bindings";
import { errorMessage } from "../format";
import type { SummaryRun } from "../summary";
import { MarkdownView } from "./MarkdownView";
import "./SummaryCard.css";

type Props = { meeting: wails.MeetingDTO; notCommitted: boolean; run: SummaryRun };

export function SummaryCard({ meeting, notCommitted, run }: Props) {
  const { pass, error, setError } = run;
  const id = meeting.id;

  const finished = meeting.state === "ready" || meeting.state === "summarized";
  const hasProject = meeting.projectKey !== "";
  const running = pass !== null;
  const disabled = !finished || !hasProject || running;
  const reason = !finished ? "Available when the transcript is finished." : !hasProject ? "Choose a project first." : "";
  const has = meeting.summaryMd !== "";

  const generate = () => {
    setError("");
    GenerateSummary(id).catch((e) => setError(errorMessage(e)));
  };

  return (
    <section className="nt-card">
      <div className="nt-card__header">Summary</div>
      {has && <MarkdownView content={meeting.summaryMd} />}
      <div className="nt-summary__actions">
        <button className={"nt-btn" + (has ? "" : " nt-btn--primary")} disabled={disabled} onClick={generate}>
          {has ? "Generate again" : "Generate summary"}
        </button>
      </div>
      {reason && <p className="nt-summary__note">{reason}</p>}
      {running && (
        <div className="nt-summary__busy">
          <span>{pass === "summary" ? "Claude Code is writing the summary…" : "Claude Code is writing the suggestions…"}</span>
          <div className="nt-bar nt-bar--indeterminate">
            <div className="nt-bar__fill" />
          </div>
        </div>
      )}
      {error && (
        <div className="nt-card nt-card--error nt-summary__error" role="alert">
          {error}
        </div>
      )}
      {notCommitted && <p className="nt-summary__note">The files are in the archive folder, not committed.</p>}
    </section>
  );
}
