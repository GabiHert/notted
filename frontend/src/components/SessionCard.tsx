import { useEffect, useRef, useState } from "react";
import { GetSettings, StartSession, StopSession } from "../api/bindings";
import { errorMessage } from "../format";
import { TerminalPane } from "./TerminalPane";
import "./SessionCard.css";

type Phase = "closed" | "open" | "ended";

// The backend keys a session by meeting id, not by instance, so StartSession and StopSession for
// one meeting run strictly in the order they were issued: a stop always ends the instance started
// before it, and a later start always waits for that stop. Idle meetings run their call at once.
const tails = new Map<string, Promise<void>>();

function inOrder(meetingId: string, op: () => Promise<void>): Promise<void> {
  const prev = tails.get(meetingId);
  const result = prev ? prev.then(op) : new Promise<void>((resolve) => resolve(op()));
  const tail = result.then(
    () => {},
    () => {},
  );
  tails.set(meetingId, tail);
  void tail.then(() => {
    if (tails.get(meetingId) === tail) tails.delete(meetingId);
  });
  return result;
}

const stopSession = (meetingId: string) => void inOrder(meetingId, () => StopSession(meetingId)).catch(() => {});

export function SessionCard({ meetingId, hasSession }: { meetingId: string; hasSession: boolean }) {
  const [checkoutPath, setCheckoutPath] = useState<string | null>(null);
  const [phase, setPhase] = useState<Phase>("closed");
  const [error, setError] = useState("");
  const [exitCode, setExitCode] = useState(0);
  const [run, setRun] = useState(0);
  const [synced, setSynced] = useState(0);
  const phaseRef = useRef<Phase>("closed");
  phaseRef.current = phase;
  const startRef = useRef({ live: false });
  // The run whose StartSession has been sent; the terminal ignores events from earlier instances.
  const sentRun = useRef(0);
  const idRef = useRef(meetingId);
  idRef.current = meetingId;

  useEffect(() => {
    let live = true;
    GetSettings()
      .then((s) => live && setCheckoutPath(s.checkoutPath))
      .catch((e) => live && setError(errorMessage(e)));
    return () => {
      live = false;
    };
  }, []);

  // Runs after the pane's own effects, so the pane is subscribed before the session starts.
  useEffect(() => {
    if (run === 0) return;
    const token = { live: true };
    startRef.current = token;
    inOrder(meetingId, () => {
      // Closed or left before its turn: the stop queued behind it has nothing to end.
      if (!token.live) return Promise.resolve();
      sentRun.current = run;
      return StartSession(meetingId).then(() => {
        if (token.live) setSynced((n) => n + 1);
      });
    }).catch((e) => {
      if (!token.live) return;
      setError(errorMessage(e));
      setPhase("closed");
    });
    return () => {
      token.live = false;
    };
  }, [run, meetingId]);

  useEffect(
    () => () => {
      if (phaseRef.current !== "closed") stopSession(idRef.current);
    },
    [],
  );

  const start = () => {
    setError("");
    setSynced(0);
    setPhase("open");
    setRun((n) => n + 1);
  };
  const close = () => {
    startRef.current.live = false;
    setPhase("closed");
    setError("");
    stopSession(meetingId);
  };

  const reason = !hasSession
    ? "This meeting has no Claude Code session yet. Generate the summary first."
    : checkoutPath === "" ? "Set the meetings archive folder in Settings > Instructions first." : "";
  const disabled = reason !== "" || checkoutPath === null;
  const showTerminal = phase !== "closed";

  return (
    <section className="nt-card">
      <div className="nt-card__header">Claude Code session</div>
      {!showTerminal && (
        <>
          <p className="nt-session__text">Continue this meeting's Claude Code conversation here. Claude asks before it changes anything, and your answer is the yes.</p>
          <div className="nt-session__actions">
            <button className="nt-btn nt-btn--primary" disabled={disabled} onClick={start}>
              Open session
            </button>
          </div>
          {reason && <p className="nt-session__note">{reason}</p>}
        </>
      )}
      {error && (
        <div className="nt-card nt-card--error nt-session__error" role="alert">
          {error}
        </div>
      )}
      {showTerminal && (
        <>
          <div className="nt-session__bar">
            <button className="nt-btn" onClick={close}>
              Close session
            </button>
          </div>
          <TerminalPane
            key={run}
            meetingId={meetingId}
            sizeSyncKey={synced}
            accepts={() => sentRun.current === run}
            onExit={(code) => {
              setExitCode(code);
              setPhase("ended");
            }}
          />
          {phase === "ended" && (
            <div className="nt-session__ended">
              <span>{`The session ended (exit code ${exitCode}).`}</span>
              <button className="nt-btn nt-btn--primary" onClick={start}>
                Restart session
              </button>
            </div>
          )}
        </>
      )}
    </section>
  );
}
