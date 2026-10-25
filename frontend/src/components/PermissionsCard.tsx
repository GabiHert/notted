import { useCallback, useEffect, useRef, useState } from "react";
import { AskPermissions, GetPermissions, OpenPermissionSettings, type wails } from "../api/bindings";
import { errorMessage } from "../format";
import { MicTest } from "./MicTest";
import "./PermissionsCard.css";

const HELP = "Notted needs both to record a meeting: the microphone for your voice, and screen and system audio recording for everyone else. Checking never asks; only Ask can.";
const HINT = "If it is already on in System Settings, select Notted there, remove it with the minus button, then click Check again.";

const ROWS = [
  { kind: "microphone", name: "Microphone" },
  { kind: "screen", name: "Screen & System Audio Recording" },
] as const;

const LABELS: Record<string, string> = { allowed: "Allowed", denied: "Not allowed", not_asked: "Not asked yet" };

// The card checks when it opens and when the window regains focus (the person comes back from System Settings).
// Every answer is checked against the newest request, so a late one never overwrites newer data.
export function PermissionsCard() {
  const [perms, setPerms] = useState<wails.PermissionsDTO | null>(null);
  const [busy, setBusy] = useState(false);
  const [asking, setAsking] = useState(false);
  const [error, setError] = useState("");
  const request = useRef(0);
  const live = useRef(true);

  // An Ask reads the permissions after macOS prompted, so its answer is newer than any check that started meanwhile.
  const run = useCallback((call: () => Promise<wails.PermissionsDTO>, ask = false) => {
    const id = ++request.current;
    setBusy(true);
    if (ask) setAsking(true);
    const current = () => live.current && (ask || request.current === id);
    call()
      .then((p) => {
        if (!current()) return;
        setPerms(p);
        setError("");
      })
      .catch((e) => {
        if (!current()) return;
        setPerms(null);
        setError(errorMessage(e));
      })
      .finally(() => {
        if (!live.current) return;
        if (ask) setAsking(false);
        if (request.current === id) setBusy(false);
      });
  }, []);

  const check = useCallback(() => run(GetPermissions), [run]);

  useEffect(() => {
    live.current = true;
    check();
    window.addEventListener("focus", check);
    return () => {
      live.current = false;
      request.current = -1;
      window.removeEventListener("focus", check);
    };
  }, [check]);

  const openSettings = (kind: string) => {
    OpenPermissionSettings(kind).catch((e) => {
      if (live.current) setError(errorMessage(e));
    });
  };

  return (
    <section className="nt-card nt-perms">
      <div className="nt-card__header">Permissions</div>
      <p className="nt-perms__help">{HELP}</p>
      <ul className="nt-perms__list">
        {ROWS.map(({ kind, name }) => {
          const state = perms?.[kind] ?? "";
          return (
            <li key={kind} className="nt-perms__row" aria-label={name}>
              <div className="nt-perms__line">
                <span className="nt-perms__name">{name}</span>
                {state && <span className={`nt-perms__status nt-perms__status--${state}`}>{LABELS[state] ?? state}</span>}
                <span className="nt-perms__buttons">
                  {state === "not_asked" && (
                    <button type="button" className="nt-btn" onClick={() => run(AskPermissions, true)} disabled={busy || asking}>
                      Ask
                    </button>
                  )}
                  <button type="button" className="nt-btn" onClick={() => openSettings(kind)}>
                    Open System Settings
                  </button>
                </span>
              </div>
              {state === "denied" && <p className="nt-perms__hint">{HINT}</p>}
              {kind === "microphone" && <MicTest />}
            </li>
          );
        })}
      </ul>
      <div className="nt-perms__actions">
        <button type="button" className="nt-btn" onClick={check} disabled={busy}>
          Check again
        </button>
      </div>
      {error && (
        <div className="nt-card nt-card--error nt-perms__error" role="alert">
          {error}
        </div>
      )}
    </section>
  );
}
