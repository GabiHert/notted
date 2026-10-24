import { useEffect, useRef, useState } from "react";
import { CommitAndPushMeeting, EventsOn } from "../api/bindings";
import { errorMessage } from "../format";
import "./SaveDialog.css";

type Ready = { meetingId: string; folder: string; files: string[] };
type Phase = "ask" | "pending" | "done" | "failed";

const lastSegment = (p: string) => p.split("/").filter(Boolean).pop() ?? p;

export function SaveDialog({ meetingId, onDeclined }: { meetingId: string; onDeclined: () => void }) {
  const [ready, setReady] = useState<Ready | null>(null);
  const [phase, setPhase] = useState<Phase>("ask");
  const [commit, setCommit] = useState("");
  const [error, setError] = useState("");
  const panel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setReady(null);
    return EventsOn("export:ready", (e: Ready) => {
      if (e.meetingId !== meetingId) return;
      setReady(e);
      setPhase("ask");
      setError("");
      setCommit("");
    });
  }, [meetingId]);

  // Keep focus inside the panel whenever the phase changes: the focused button may have unmounted or been disabled.
  useEffect(() => {
    if (!ready) return;
    const el = panel.current;
    if (!el) return;
    const pick = phase === "failed" ? "button:last-of-type" : "button:not(:disabled)";
    (el.querySelector<HTMLElement>(pick) ?? el).focus();
  }, [ready, phase]);

  const open = ready !== null;
  const decline = () => {
    setReady(null);
    onDeclined();
  };
  const close = () => setReady(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        if (phase === "ask") decline();
        else if (phase !== "pending") close();
        return;
      }
      if (e.key !== "Tab") return;
      const items = [...(panel.current?.querySelectorAll<HTMLElement>("button:not(:disabled)") ?? [])];
      if (items.length === 0) {
        e.preventDefault();
        panel.current?.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      const inside = panel.current?.contains(active) ?? false;
      if (e.shiftKey && (active === first || !inside)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (active === last || !inside)) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, phase, onDeclined]);

  if (!ready) return null;

  const run = () => {
    setPhase("pending");
    setError("");
    CommitAndPushMeeting(meetingId)
      .then((r) => {
        setCommit(r.commit);
        setPhase("done");
      })
      .catch((e) => {
        setError(errorMessage(e));
        setPhase("failed");
      });
  };

  return (
    <div className="nt-overlay">
      <div className="nt-dialog" role="dialog" aria-modal="true" aria-labelledby="nt-save-title" ref={panel} tabIndex={-1}>
        <h3 id="nt-save-title">Save this meeting to git?</h3>
        {phase === "done" ? (
          <>
            <p className="nt-dialog__text">{`Committed and pushed: ${commit}`}</p>
            <div className="nt-dialog__actions">
              <button className="nt-btn nt-btn--primary" onClick={close}>
                Close
              </button>
            </div>
          </>
        ) : (
          <>
            <p className="nt-dialog__text">{`Notted wrote these files to ${ready.folder} in the archive folder:`}</p>
            <ul className="nt-dialog__files">
              {ready.files.map((f) => (
                <li key={f}>{lastSegment(f)}</li>
              ))}
            </ul>
            <p className="nt-dialog__hint">Nothing is staged or committed until you choose.</p>
            {phase === "failed" && (
              <div className="nt-card nt-card--error nt-dialog__error" role="alert">
                {error}
              </div>
            )}
            <div className="nt-dialog__actions">
              {phase === "failed" ? (
                <>
                  <button className="nt-btn" onClick={close}>
                    Close
                  </button>
                  <button className="nt-btn nt-btn--primary" onClick={run}>
                    Try again
                  </button>
                </>
              ) : (
                <>
                  <button className="nt-btn" onClick={decline} disabled={phase === "pending"}>
                    Not now
                  </button>
                  <button className="nt-btn nt-btn--primary" onClick={run} disabled={phase === "pending"}>
                    {phase === "pending" ? "Committing…" : "Commit and push"}
                  </button>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
