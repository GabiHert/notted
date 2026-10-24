import { useEffect, useState } from "react";
import { EventsOn, GenerateSummary, type wails } from "../api/bindings";
import { errorMessage } from "../format";
import { MarkdownView } from "./MarkdownView";
import "./SummaryCard.css";

type Props = { meeting: wails.MeetingDTO; notCommitted: boolean; onRefresh: () => void };
type SummaryState = { meetingId: string; pass: "summary" | "suggestions"; state: "running" | "done" | "error"; message?: string };

export function SummaryCard({ meeting, notCommitted, onRefresh }: Props) {
  const [pass, setPass] = useState<"summary" | "suggestions" | null>(null);
  const [error, setError] = useState("");
  const id = meeting.id;

  useEffect(() => {
    setPass(null);
    setError("");
    return EventsOn("summary:state", (e: SummaryState) => {
      if (e.meetingId !== id) return;
      if (e.state === "running") {
        setError("");
        setPass(e.pass);
      } else if (e.state === "error") {
        setPass(null);
        setError(e.message ?? "");
      } else {
        setPass(null);
        onRefresh();
      }
    });
  }, [id, onRefresh]);

  const finished = meeting.state === "ready" || meeting.state === "summarized";
  const hasClient = meeting.clientKey !== "";
  const running = pass !== null;
  const disabled = !finished || !hasClient || running;
  const reason = !finished ? "Available when the transcript is finished." : !hasClient ? "Choose a client first." : "";
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
