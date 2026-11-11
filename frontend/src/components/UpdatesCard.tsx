// UpdatesCard: the Settings card for the release updater. From the release-updater skill, restyled
// for Phonon. `api` is wired to the App bindings and `on` to EventsOn (see SettingsView).
import { useEffect, useRef, useState } from "react";
import "./UpdatesCard.css";

export type UpdateRelease = { tag: string; name: string; notes: string; date: number; url: string };
export type UpdateStatus = {
  version: string;
  repo: string;
  latest: UpdateRelease | null;
  upToDate: boolean;
  hasToken: boolean;
  state: "idle" | "checking" | "downloading" | "verifying" | "installing" | "failed" | string;
  message: string;
  lines: string[];
  seq: number;
  installedApp: string;
};

export type UpdatesAPI = {
  getStatus: () => Promise<UpdateStatus>;
  check: () => Promise<UpdateStatus>;
  start: () => Promise<void>;
  setToken: (token: string) => Promise<UpdateStatus>;
};

const RUNNING = ["checking", "downloading", "verifying", "installing"];
const MAX_LINES = 5000;

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

export function UpdatesCard({
  appName,
  api,
  on,
}: {
  appName: string;
  api: UpdatesAPI;
  on: (event: string, cb: (data: any) => void) => () => void;
}) {
  const [u, setU] = useState<UpdateStatus | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const [token, setToken] = useState("");
  const [tokenOpen, setTokenOpen] = useState(false);
  const lastSeq = useRef(0);
  const logRef = useRef<HTMLPreElement>(null);

  const load = (s: UpdateStatus) => {
    setU(s);
    if (s.seq >= lastSeq.current) {
      setLines(s.lines ?? []);
      lastSeq.current = s.seq;
    }
  };
  const refresh = () => api.getStatus().then(load).catch((e) => setErr(errText(e)));

  useEffect(() => {
    refresh();
    const offOut = on("update:output", (e: { seq: number; line: string }) => {
      if (!e || e.seq <= lastSeq.current) return;
      lastSeq.current = e.seq;
      setLines((l) => (l.length >= MAX_LINES ? [...l.slice(l.length - MAX_LINES + 1), e.line] : [...l, e.line]));
    });
    const offState = on("update:state", (e: { state: string; message: string }) => {
      if (!e) return;
      if (e.state === "checking") setLines([]);
      setU((cur) => (cur ? { ...cur, state: e.state, message: e.message ?? "" } : cur));
      if (e.state === "idle" || e.state === "failed") refresh();
    });
    const offAvail = on("update:available", () => refresh());
    return () => {
      offOut();
      offState();
      offAvail();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  const act = async (what: string, f: () => Promise<unknown>) => {
    setBusy(what);
    setErr("");
    try {
      await f();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy("");
    }
  };

  const state = u?.state ?? "idle";
  const running = RUNNING.includes(state);
  const latest = u?.latest;
  const translocated = !!u?.installedApp?.includes("/AppTranslocation/");
  const stateText: Record<string, string> = {
    checking: "Checking GitHub…",
    downloading: "Downloading…",
    verifying: "Checking the download and its signature…",
    installing: u?.message || `${appName} quits now and reopens when the new version is in place.`,
  };

  return (
    <section className="nt-card nt-update" aria-labelledby="nt-update-title">
      <div className="nt-card__header" id="nt-update-title">
        Updates
      </div>
      <p className="nt-update__help">
        Running <code>{u?.version || "—"}</code>.{" "}
        {latest && !running && state !== "failed" &&
          (u?.upToDate ? (
            <span className="nt-update__ok">{appName} is up to date.</span>
          ) : (
            <span>
              New version: <code>{latest.tag}</code>
              {latest.name && latest.name !== latest.tag ? ` · ${latest.name}` : ""}
            </span>
          ))}
      </p>
      {translocated && (
        <div className="nt-card nt-card--error nt-update__error" role="alert">
          macOS is running {appName} from a temporary read-only copy, so it can't update itself. Quit {appName}, drag {appName}.app into
          Applications in Finder, then open it from there.
        </div>
      )}
      {u && !u.installedApp && <p className="nt-update__help">{appName} is not running from an app bundle, so it cannot update itself.</p>}
      <div className="nt-update__actions">
        <button type="button" className="nt-btn" disabled={!!busy || running} onClick={() => act("check", async () => load(await api.check()))}>
          {busy === "check" ? "Checking…" : "Check for updates"}
        </button>
        {latest && !u?.upToDate && (
          <button
            type="button"
            className="nt-btn nt-btn--primary"
            disabled={!!busy || running || !u?.installedApp || translocated}
            onClick={() =>
              act("update", async () => {
                await api.start();
                await refresh();
              })
            }
          >
            {running ? "Updating…" : `Update ${appName}`}
          </button>
        )}
        <span className="nt-update__status" role="status">
          {running ? stateText[state] : ""}
        </span>
      </div>
      {running && (
        <div className="nt-bar nt-bar--indeterminate">
          <div className="nt-bar__fill" />
        </div>
      )}
      {(err || (state === "failed" && u?.message)) && (
        <div className="nt-card nt-card--error nt-update__error" role="alert">
          {err || u?.message}
        </div>
      )}
      <p className="nt-update__help nt-update__source">
        Updates come from the releases of <code>{u?.repo}</code>, checked at launch and every 6 hours. Each download's checksum and code
        signature are checked before {appName} replaces itself.{" "}
        {u?.hasToken ? "A GitHub token is saved in your keychain." : "A private repo needs a fine-grained token with read-only Contents access."}{" "}
        <button type="button" className="nt-link" onClick={() => setTokenOpen((o) => !o)} aria-expanded={tokenOpen}>
          {u?.hasToken ? "Change token" : "Add token"}
        </button>
      </p>
      {tokenOpen && (
        <form
          className="nt-update__token"
          onSubmit={(e) => {
            e.preventDefault();
            act("token", async () => {
              load(await api.setToken(token));
              setToken("");
              setTokenOpen(false);
            });
          }}
        >
          <input
            type="password"
            autoComplete="off"
            spellCheck={false}
            placeholder="github_pat_…"
            aria-label="GitHub token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
          <button type="submit" className="nt-btn" disabled={!!busy || !token.trim()}>
            Save in keychain
          </button>
          {u?.hasToken && (
            <button type="button" className="nt-btn" disabled={!!busy} onClick={() => act("token", async () => load(await api.setToken("")))}>
              Remove
            </button>
          )}
        </form>
      )}
      {lines.length > 0 && (
        <details className="nt-update__details" open={running || state === "failed"}>
          <summary>Output ({lines.length} lines)</summary>
          <pre ref={logRef} className="nt-update__out" data-testid="update-output">
            {lines.join("\n")}
          </pre>
        </details>
      )}
    </section>
  );
}
