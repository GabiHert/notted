import { useEffect, useRef, useState } from "react";
import { ChooseFolder, EventsOn, GetUpdateStatus, SetUpdateSource, StartUpdate } from "../api/bindings";
import { errorMessage } from "../format";
import "./UpdateCard.css";

const HELP = "Notted pulls the main branch of your Notted source folder, builds it, and reopens as the new version. Nothing runs until you click Update Notted.";
const SIGNED = 'Builds are signed with your "Notted Local Signing" certificate, so macOS keeps the microphone and screen recording permissions after an update.';
const UNSIGNED = 'No "Notted Local Signing" certificate in your keychain, so builds are signed ad hoc and macOS asks for the microphone and screen recording permissions again after each update. To stop that, run bash scripts/create-signing-identity.sh once in your Notted source folder.';
const NOT_BUNDLE = "Notted is not running from an app bundle, so it cannot update itself.";
const PICKER_TITLE = "Choose the Notted source folder";
const MAX_LINES = 5000;

type OutputEvent = { seq: number; line: string };
type StateEvent = { state: string; message?: string };

// The card loads its own state and listens for the run's events. Late answers after unmount are dropped,
// a stale output line is dropped by its sequence number, and every busy flag is released in `finally`.
export function UpdateCard() {
  const [loaded, setLoaded] = useState(false);
  const [source, setSource] = useState("");
  const [installedApp, setInstalledApp] = useState("");
  const [signing, setSigning] = useState(false);
  const [state, setState] = useState("idle");
  const [message, setMessage] = useState("");
  const [lines, setLines] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const lastSeq = useRef(0);
  const live = useRef(true);
  const request = useRef(0);
  const stateEvents = useRef(0);
  const pullEvents = useRef(0);
  const busyRef = useRef(false);
  const outRef = useRef<HTMLPreElement>(null);
  const stick = useRef(true);
  const refocus = useRef<(() => void) | null>(null);

  const running = state === "pulling" || state === "building" || state === "installing";

  const refresh = () => {
    const id = ++request.current;
    const seen = stateEvents.current;
    const pulls = pullEvents.current;
    GetUpdateStatus()
      .then((status) => {
        if (request.current !== id || !live.current) return;
        setLoaded(true);
        setSource(status.sourcePath);
        setInstalledApp(status.installedApp);
        setSigning(status.signing);
        // An event that arrived while this call was in flight is newer than the status.
        if (stateEvents.current === seen) {
          setState(status.state);
          setMessage(status.message);
        }
        const last = lastSeq.current;
        if (status.seq >= last) {
          setLines(status.lines ?? []);
          lastSeq.current = status.seq;
        } else if (pullEvents.current === pulls) {
          // Lines newer than the status arrived while the call was in flight: keep them after the status lines.
          const newer = last - status.seq;
          setLines((l) => {
            const n = [...(status.lines ?? []), ...l.slice(Math.max(0, l.length - newer))];
            return n.length > MAX_LINES ? n.slice(n.length - MAX_LINES) : n;
          });
        }
        // Otherwise a new run started during the call: the status lines belong to the previous run.
      })
      .catch((e) => {
        if (request.current !== id || !live.current) return;
        setError(errorMessage(e));
      });
  };

  useEffect(() => {
    live.current = true;
    const offOutput = EventsOn("update:output", (e: OutputEvent) => {
      if (!e || e.seq <= lastSeq.current) return;
      lastSeq.current = e.seq;
      setLines((l) => {
        const n = [...l, e.line];
        return n.length > MAX_LINES ? n.slice(n.length - MAX_LINES) : n;
      });
    });
    const offState = EventsOn("update:state", (e: StateEvent) => {
      if (!e) return;
      stateEvents.current++;
      setState(e.state);
      setMessage(e.message ?? "");
      if (e.state === "pulling") {
        pullEvents.current++;
        setLines([]);
        setError("");
      }
    });
    refresh();
    return () => {
      offOutput?.();
      offState?.();
      live.current = false;
      request.current = -1;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    // Wait for the output block to exist: it renders only once the run's first event arrives.
    if (!outRef.current) return;
    refocus.current?.();
    refocus.current = null;
  });

  useEffect(() => {
    const el = outRef.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [lines]);

  const choose = async () => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    refocus.current = null;
    try {
      const p = await ChooseFolder(PICKER_TITLE, source);
      if (!live.current || p === "") return;
      await SetUpdateSource(p);
      if (!live.current) return;
      refresh();
    } catch (e) {
      if (live.current) setError(errorMessage(e));
    } finally {
      busyRef.current = false;
      if (live.current) setBusy(false);
    }
  };

  const start = async () => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      await StartUpdate();
      if (!live.current) return;
      refocus.current = () => outRef.current?.focus();
      refresh();
    } catch (e) {
      if (live.current) setError(errorMessage(e));
    } finally {
      busyRef.current = false;
      if (live.current) setBusy(false);
    }
  };

  const statusText = state === "pulling" ? "Pulling main…" : state === "building" ? "Building… This takes a few minutes." : state === "installing" ? message : "";
  const shownError = error || (state === "failed" ? message : "");

  return (
    <section className="nt-card nt-update">
      <div className="nt-card__header">Update Notted</div>
      <p className="nt-update__help">{HELP}</p>
      <label htmlFor="nt-update-source">Notted source folder</label>
      <div className="nt-update__path">
        <input id="nt-update-source" readOnly value={source} placeholder="Choose the folder of your Notted clone" />
        <button type="button" className="nt-btn" aria-label={PICKER_TITLE} onClick={choose} disabled={!loaded || busy || running}>
          Choose…
        </button>
      </div>
      {loaded && <p className="nt-update__help">{signing ? SIGNED : UNSIGNED}</p>}
      {loaded && <p className="nt-update__help">{installedApp ? `Notted replaces ${installedApp} and reopens it.` : NOT_BUNDLE}</p>}
      <div className="nt-update__actions">
        <button type="button" className="nt-btn nt-btn--primary" onClick={start} disabled={!loaded || busy || running || !source || !installedApp}>
          {running ? "Updating…" : "Update Notted"}
        </button>
        <span className="nt-update__status" role="status">
          {statusText}
        </span>
      </div>
      {running && (
        <div className="nt-bar nt-bar--indeterminate">
          <div className="nt-bar__fill" />
        </div>
      )}
      {shownError && (
        <div className="nt-card nt-card--error nt-update__error" role="alert">
          {shownError}
        </div>
      )}
      {(running || state === "failed" || lines.length > 0) && (
        <pre
          ref={outRef}
          role="region"
          className="nt-update__out"
          tabIndex={0}
          aria-label="Update output"
          data-testid="update-output"
          onScroll={() => {
            const el = outRef.current!;
            stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
          }}
        >
          {lines.join("\n")}
        </pre>
      )}
    </section>
  );
}
