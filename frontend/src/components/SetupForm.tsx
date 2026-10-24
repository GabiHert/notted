import { useEffect, useRef, useState } from "react";
import { GetSettings, SetSettings } from "../api/bindings";
import { errorMessage } from "../format";
import "./SetupForm.css";

type Props = { clients: string[]; clientsError: string; onSaved: () => void };

export function SetupForm({ clients, clientsError, onSaved }: Props) {
  const [loaded, setLoaded] = useState(false);
  const [profile, setProfile] = useState("");
  const [checkoutPath, setCheckoutPath] = useState("");
  const [roots, setRoots] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const live = useRef(true);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => {
    live.current = true;
    GetSettings()
      .then((s) => {
        if (!live.current) return;
        setProfile(s.profile ?? "");
        setCheckoutPath(s.checkoutPath ?? "");
        setRoots({ ...(s.projectRoots ?? {}) });
        setLoaded(true);
      })
      .catch((e) => {
        if (!live.current) return;
        setError(errorMessage(e));
        setLoaded(true);
      });
    return () => {
      live.current = false;
      window.clearTimeout(timer.current);
    };
  }, []);

  const save = () => {
    const projectRoots = { ...roots };
    for (const c of clients) {
      const v = (roots[c] ?? "").trim();
      if (v) projectRoots[c] = v;
      else delete projectRoots[c];
    }
    setSaving(true);
    setError("");
    setNotice("");
    window.clearTimeout(timer.current);
    SetSettings({ profile: profile.trim(), checkoutPath: checkoutPath.trim(), projectRoots })
      .then(() => {
        if (!live.current) return;
        setSaving(false);
        setNotice("Settings saved.");
        timer.current = window.setTimeout(() => setNotice(""), 3000);
        onSaved();
      })
      .catch((e) => {
        if (!live.current) return;
        setSaving(false);
        setError(errorMessage(e));
      });
  };

  return (
    <section className="nt-card">
      <div className="nt-card__header">Your setup</div>
      {loaded && (
        <>
          <div className="nt-setup__field">
            <label htmlFor="nt-profile">Profile</label>
            <input id="nt-profile" value={profile} onChange={(e) => setProfile(e.target.value)} />
            <span className="nt-setup__help">Your file in instructions/people/, without .md (for example ana).</span>
          </div>
          <div className="nt-setup__field">
            <label htmlFor="nt-checkout">Archive folder</label>
            <input id="nt-checkout" value={checkoutPath} onChange={(e) => setCheckoutPath(e.target.value)} />
            <span className="nt-setup__help">The full path of your local clone of the meetings archive.</span>
          </div>
          <div className="nt-setup__sub">Project roots</div>
          <span className="nt-setup__help">Optional. A folder per client that Claude Code may read for that client's meetings.</span>
          {clientsError && <p className="nt-setup__help nt-setup__help--block">{clientsError}</p>}
          {clients.map((c) => (
            <div className="nt-setup__root" key={c}>
              <label htmlFor={"nt-root-" + c}>{c}</label>
              <input id={"nt-root-" + c} placeholder="/full/path/to/the/project" value={roots[c] ?? ""} onChange={(e) => setRoots({ ...roots, [c]: e.target.value })} />
            </div>
          ))}
          {error && (
            <div className="nt-card nt-card--error nt-setup__error" role="alert">
              {error}
            </div>
          )}
          <div className="nt-setup__actions">
            <button className="nt-btn nt-btn--primary" onClick={save} disabled={saving}>
              Save settings
            </button>
            {notice && <span className="nt-setup__notice">{notice}</span>}
          </div>
        </>
      )}
    </section>
  );
}
