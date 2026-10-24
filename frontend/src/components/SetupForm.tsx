import { useEffect, useRef, useState } from "react";
import { ChooseFolder, CloneArchive, CreateProfile, CreateProject, SetSettings, type wails } from "../api/bindings";
import { errorMessage } from "../format";
import "./SetupForm.css";

type Props = {
  saved: wails.SettingsDTO | null;
  settingsError: string;
  projects: string[];
  projectsError: string;
  profiles: string[];
  profilesError: string;
  onSaved: () => void;
  onOpen: (path: string) => void;
};

const NEW_PROFILE = "__new__";
const ARCHIVE_TITLE = "Choose the meetings archive folder";
const CLONE_TITLE = "Choose where to clone the archive";

function Choose({ label, onClick, disabled }: { label: string; onClick: () => void; disabled: boolean }) {
  return (
    <button type="button" className="nt-btn" aria-label={label} onClick={onClick} disabled={disabled}>
      Choose…
    </button>
  );
}

function ErrorCard({ text }: { text: string }) {
  return (
    <div className="nt-card nt-card--error nt-setup__error" role="alert">
      {text}
    </div>
  );
}

// One busy flag covers every call in the card, so nothing can run twice or overlap.
// A result that arrives after unmount, or after its inline form was closed, is dropped.
export function SetupForm({ saved, settingsError, projects, projectsError, profiles, profilesError, onSaved, onOpen }: Props) {
  const [checkoutPath, setCheckoutPath] = useState(saved?.checkoutPath ?? "");
  const [roots, setRoots] = useState<Record<string, string>>({ ...(saved?.projectRoots ?? {}) });
  const [profile, setProfile] = useState(saved?.profile ?? "");
  const [busy, setBusy] = useState<"" | "clone" | "other">("");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  const [cloneOpen, setCloneOpen] = useState(false);
  const [cloneAddress, setCloneAddress] = useState("");
  const [cloneParent, setCloneParent] = useState("");
  const [cloneError, setCloneError] = useState("");

  const [profileOpen, setProfileOpen] = useState(false);
  const [newProfile, setNewProfile] = useState("");
  const [profileError, setProfileError] = useState("");

  const [projectOpen, setProjectOpen] = useState(false);
  const [newProject, setNewProject] = useState("");
  const [projectError, setProjectError] = useState("");

  const live = useRef(true);
  const prevSaved = useRef(saved);
  const busyRef = useRef(false);
  const epoch = useRef(0);
  const timer = useRef<number | undefined>(undefined);
  const refocus = useRef<(() => void) | null>(null);
  const archiveInput = useRef<HTMLInputElement>(null);
  const cloneLink = useRef<HTMLButtonElement>(null);
  const profileSelect = useRef<HTMLSelectElement>(null);
  const projectButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
      window.clearTimeout(timer.current);
    };
  }, []);

  // A refreshed `saved` updates only the fields the person has not changed: a field whose text still
  // equals the previous saved value (or already equals the new one) takes the new value; the rest keep their text.
  useEffect(() => {
    const old = prevSaved.current;
    prevSaved.current = saved;
    const follows = (text: string, before: string, after: string) => text === before || text.trim() === after;
    setCheckoutPath((c) => (follows(c, old?.checkoutPath ?? "", saved?.checkoutPath ?? "") ? (saved?.checkoutPath ?? "") : c));
    setProfile((p) => (p === (old?.profile ?? "") || p === (saved?.profile ?? "") ? (saved?.profile ?? "") : p));
    setRoots((r) => {
      const next = { ...r };
      const keys = new Set([...Object.keys(r), ...Object.keys(old?.projectRoots ?? {}), ...Object.keys(saved?.projectRoots ?? {})]);
      for (const k of keys) {
        const after = saved?.projectRoots?.[k] ?? "";
        if (follows(r[k] ?? "", old?.projectRoots?.[k] ?? "", after)) {
          if (after) next[k] = after;
          else delete next[k];
        }
      }
      return next;
    });
  }, [saved]);

  useEffect(() => {
    refocus.current?.();
    refocus.current = null;
  });

  const hasArchive = Boolean(saved?.checkoutPath);
  const hasProfile = Boolean(saved?.profile);
  const base = () => ({ profile: saved?.profile ?? "", checkoutPath: saved?.checkoutPath ?? "", projectRoots: { ...(saved?.projectRoots ?? {}) } });

  const begin = (kind: "clone" | "other"): boolean => {
    if (busyRef.current) return false;
    busyRef.current = true;
    setBusy(kind);
    window.clearTimeout(timer.current);
    setNotice("");
    setError("");
    setCloneError("");
    setProfileError("");
    setProjectError("");
    return true;
  };
  const end = () => {
    busyRef.current = false;
    if (live.current) setBusy("");
  };
  const announceSaved = () => {
    setNotice("Settings saved.");
    timer.current = window.setTimeout(() => setNotice(""), 3000);
  };
  const fail = (set: (m: string) => void) => (e: unknown) => {
    if (live.current) set(errorMessage(e));
  };

  // An explicit pick: SetSettings with the last saved settings plus that one field.
  const saveNow = async (patch: Partial<wails.SettingsDTO>) => {
    await SetSettings({ ...base(), ...patch } as wails.SettingsDTO);
    if (!live.current) return false;
    announceSaved();
    onSaved();
    return true;
  };

  const pickArchive = async () => {
    if (!begin("other")) return;
    try {
      const path = await ChooseFolder(ARCHIVE_TITLE, checkoutPath);
      if (!live.current || !path) return;
      setCheckoutPath(path);
      await saveNow({ checkoutPath: path });
    } catch (e) {
      fail(setError)(e);
    } finally {
      end();
    }
  };

  const pickRoot = async (key: string) => {
    if (!begin("other")) return;
    try {
      const path = await ChooseFolder("Choose the root folder for " + key, roots[key] ?? "");
      if (!live.current || !path) return;
      setRoots((r) => ({ ...r, [key]: path }));
      await saveNow({ projectRoots: { ...(saved?.projectRoots ?? {}), [key]: path } });
    } catch (e) {
      fail(setError)(e);
    } finally {
      end();
    }
  };

  const saveTyped = async () => {
    if (!begin("other")) return;
    const projectRoots = { ...roots };
    for (const key of projects) {
      const v = (roots[key] ?? "").trim();
      if (v) projectRoots[key] = v;
      else delete projectRoots[key];
    }
    try {
      await saveNow({ checkoutPath: checkoutPath.trim(), projectRoots });
    } catch (e) {
      fail(setError)(e);
    } finally {
      end();
    }
  };

  const closeClone = () => {
    epoch.current++;
    setCloneOpen(false);
    setCloneAddress("");
    setCloneParent("");
    setCloneError("");
  };
  const pickCloneParent = async () => {
    if (!begin("other")) return;
    const mine = epoch.current;
    try {
      const path = await ChooseFolder(CLONE_TITLE, cloneParent);
      if (!live.current || epoch.current !== mine || !path) return;
      setCloneParent(path);
    } catch (e) {
      if (epoch.current === mine) fail(setCloneError)(e);
    } finally {
      end();
    }
  };
  const clone = async () => {
    if (!begin("clone")) return;
    const mine = epoch.current;
    try {
      const path = await CloneArchive(cloneAddress.trim(), cloneParent.trim());
      if (!live.current || epoch.current !== mine) return;
      setCheckoutPath(path);
      await SetSettings({ ...base(), checkoutPath: path });
      if (!live.current) return;
      closeClone();
      refocus.current = () => archiveInput.current?.focus();
      setNotice(`Cloned into ${path}. It is now your archive folder.`);
      onSaved();
    } catch (e) {
      if (epoch.current === mine) fail(setCloneError)(e);
    } finally {
      end();
    }
  };

  const closeProfile = () => {
    epoch.current++;
    setProfileOpen(false);
    setNewProfile("");
    setProfileError("");
    refocus.current = () => profileSelect.current?.focus();
  };
  const chooseProfile = async (value: string) => {
    if (value === NEW_PROFILE) {
      setProfileOpen(true);
      return;
    }
    if (!value || !begin("other")) return;
    setProfile(value);
    try {
      await saveNow({ profile: value });
    } catch (e) {
      fail(setError)(e);
    } finally {
      end();
    }
  };
  const createProfile = async () => {
    if (!begin("other")) return;
    const name = newProfile.trim();
    try {
      const path = await CreateProfile(name);
      if (!live.current) return;
      await SetSettings({ ...base(), profile: name });
      if (!live.current) return;
      closeProfile();
      setNotice(`Created ${path}. Fill it in below.`);
      onSaved();
      onOpen(path);
    } catch (e) {
      fail(setProfileError)(e);
    } finally {
      end();
    }
  };

  const closeProject = () => {
    epoch.current++;
    setProjectOpen(false);
    setNewProject("");
    setProjectError("");
    refocus.current = () => projectButton.current?.focus();
  };
  const createProject = async () => {
    if (!begin("other")) return;
    try {
      const paths = await CreateProject(newProject.trim());
      if (!live.current) return;
      closeProject();
      setNotice(paths.length > 1 ? `Created ${paths[0]} and ${paths[1]}. Fill them in below.` : `Created ${paths[0]}. Fill it in below.`);
      onSaved();
      if (paths.length > 0) onOpen(paths[0]);
    } catch (e) {
      fail(setProjectError)(e);
    } finally {
      end();
    }
  };

  const loaded = saved !== null || settingsError !== "";
  const extra = [saved?.profile ?? "", profile].filter((n, i, a) => n && !profiles.includes(n) && a.indexOf(n) === i);
  const names = [...profiles, ...extra];
  const shownError = error || settingsError;

  return (
    <section className="nt-card">
      <div className="nt-card__header">Your setup</div>
      {loaded && (
        <>
          <div className="nt-setup__section">
            <label htmlFor="nt-checkout">Archive folder</label>
            <div className="nt-setup__path">
              <input id="nt-checkout" ref={archiveInput} value={checkoutPath} onChange={(e) => setCheckoutPath(e.target.value)} />
              <Choose label={ARCHIVE_TITLE} onClick={pickArchive} disabled={busy !== ""} />
            </div>
            <span className="nt-setup__help">The full path of your local clone of the meetings archive.</span>
            {!cloneOpen && (
              <div>
                <button
                  type="button"
                  ref={cloneLink}
                  className="nt-btn--link"
                  onClick={() => {
                    setCloneOpen(true);
                  }}
                >
                  Clone the archive from GitHub instead
                </button>
              </div>
            )}
            {cloneOpen && (
              <div className="nt-setup__inline">
                <label htmlFor="nt-clone-address">GitHub address</label>
                <input id="nt-clone-address" autoFocus value={cloneAddress} onChange={(e) => setCloneAddress(e.target.value)} />
                <span className="nt-setup__help">The address you would give to git clone, from the Code button on the archive's GitHub page. Notted runs your own git with your own login.</span>
                <label htmlFor="nt-clone-parent">Clone into</label>
                <div className="nt-setup__path">
                  <input id="nt-clone-parent" value={cloneParent} onChange={(e) => setCloneParent(e.target.value)} />
                  <Choose label={CLONE_TITLE} onClick={pickCloneParent} disabled={busy !== ""} />
                </div>
                <span className="nt-setup__help">Notted creates the archive folder inside this folder.</span>
                {cloneError && <ErrorCard text={cloneError} />}
                <div className="nt-setup__actions">
                  <button type="button" className="nt-btn nt-btn--primary" onClick={clone} disabled={busy !== "" || !cloneAddress.trim() || !cloneParent.trim()}>
                    {busy === "clone" ? "Cloning…" : "Clone archive"}
                  </button>
                  <button
                    type="button"
                    className="nt-btn"
                    onClick={() => {
                      closeClone();
                      refocus.current = () => cloneLink.current?.focus();
                    }}
                    disabled={busy === "clone"}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>

          <div className="nt-setup__section">
            <label htmlFor="nt-profile">Profile</label>
            <select id="nt-profile" ref={profileSelect} className="nt-setup__select" value={profile} disabled={!hasArchive || busy !== ""} onChange={(e) => chooseProfile(e.target.value)}>
              <option value="" disabled>
                Choose your profile
              </option>
              {names.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
              <option value={NEW_PROFILE}>New profile…</option>
            </select>
            <span className="nt-setup__help nt-setup__help--block">{hasArchive ? "Your file in instructions/people/ of the archive." : "Choose the archive folder first."}</span>
            {profilesError && <p className="nt-setup__help nt-setup__help--block">{profilesError}</p>}
            {profileOpen && (
              <div className="nt-setup__inline">
                <label htmlFor="nt-new-profile">New profile name</label>
                <input id="nt-new-profile" autoFocus value={newProfile} onChange={(e) => setNewProfile(e.target.value)} />
                <span className="nt-setup__help">Use lowercase letters, digits and dashes, for example ana or ana-lima.</span>
                {profileError && <ErrorCard text={profileError} />}
                <div className="nt-setup__actions">
                  <button type="button" className="nt-btn nt-btn--primary" onClick={createProfile} disabled={busy !== "" || !newProfile.trim()}>
                    Create profile
                  </button>
                  <button type="button" className="nt-btn" onClick={closeProfile} disabled={busy !== ""}>
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>

          <div className="nt-setup__section">
            <div className="nt-setup__sub">Projects</div>
            <span className="nt-setup__help">Optional. A folder per project that Claude Code may read during that project's meetings.</span>
            {!hasArchive && <p className="nt-setup__help nt-setup__help--block">Choose the archive folder first.</p>}
            {hasArchive && projectsError && <p className="nt-setup__help nt-setup__help--block">{projectsError}</p>}
            {hasArchive && !projectsError && projects.length === 0 && <p className="nt-setup__help nt-setup__help--block">No projects yet. Add one with New project.</p>}
            {hasArchive &&
              projects.map((key) => (
                <div className="nt-setup__root" key={key}>
                  <label htmlFor={"nt-root-" + key}>{key}</label>
                  <input id={"nt-root-" + key} placeholder="/full/path/to/the/project" value={roots[key] ?? ""} onChange={(e) => setRoots({ ...roots, [key]: e.target.value })} />
                  <Choose label={"Choose the root folder for " + key} onClick={() => pickRoot(key)} disabled={busy !== ""} />
                </div>
              ))}
            <div className="nt-setup__new">
              <button type="button" ref={projectButton} className="nt-btn" onClick={() => setProjectOpen(true)} disabled={!hasProfile || projectOpen || busy !== ""}>
                New project…
              </button>
              {!hasProfile && <span className="nt-setup__help">Choose your profile first.</span>}
            </div>
            {projectOpen && (
              <div className="nt-setup__inline">
                <label htmlFor="nt-new-project">New project key</label>
                <input id="nt-new-project" autoFocus value={newProject} onChange={(e) => setNewProject(e.target.value)} />
                <span className="nt-setup__help">Use lowercase letters, digits and dashes, for example acme or acme-web. Notted creates the project's file and your own file for it.</span>
                {projectError && <ErrorCard text={projectError} />}
                <div className="nt-setup__actions">
                  <button type="button" className="nt-btn nt-btn--primary" onClick={createProject} disabled={busy !== "" || !newProject.trim()}>
                    Create project
                  </button>
                  <button type="button" className="nt-btn" onClick={closeProject} disabled={busy !== ""}>
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>

          {shownError && <ErrorCard text={shownError} />}
          <div className="nt-setup__actions">
            <button type="button" className="nt-btn nt-btn--primary" onClick={saveTyped} disabled={busy !== ""}>
              Save settings
            </button>
            {notice && <span className="nt-setup__notice">{notice}</span>}
          </div>
        </>
      )}
    </section>
  );
}
