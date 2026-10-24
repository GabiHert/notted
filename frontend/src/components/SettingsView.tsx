import { useCallback, useEffect, useRef, useState } from "react";
import { GetSettings, ListMyProjectKeys, ListProfiles, ListProjects, type wails } from "../api/bindings";
import { errorMessage } from "../format";
import { InstructionFiles } from "./InstructionFiles";
import { PromptPreview } from "./PromptPreview";
import { SetupForm } from "./SetupForm";
import { UpdateCard } from "./UpdateCard";
import { WelcomeCard } from "./WelcomeCard";
import "./SettingsView.css";

type OpenRequest = { path: string; seq: number };

export function SettingsView({ onDirtyPath }: { onDirtyPath?: (path: string | null) => void } = {}) {
  const [saved, setSaved] = useState<wails.SettingsDTO | null>(null);
  const [settingsError, setSettingsError] = useState("");
  const [projects, setProjects] = useState<string[]>([]);
  const [projectsError, setProjectsError] = useState("");
  const [profiles, setProfiles] = useState<string[]>([]);
  const [profilesError, setProfilesError] = useState("");
  const [myProjects, setMyProjects] = useState<string[] | null>(null);
  const [myProjectsError, setMyProjectsError] = useState("");
  const [filesRefresh, setFilesRefresh] = useState(0);
  const [openRequest, setOpenRequest] = useState<OpenRequest | null>(null);
  const request = useRef(0);
  const seq = useRef(0);

  // Every result is checked against the newest request, so a late answer never overwrites newer data.
  const reload = useCallback(() => {
    const id = ++request.current;
    const current = () => request.current === id;
    GetSettings()
      .then((s) => {
        if (!current()) return;
        setSaved(s);
        setSettingsError("");
        if (!s.checkoutPath) {
          setProjects([]);
          setProjectsError("");
          setProfiles([]);
          setProfilesError("");
          setMyProjects(null);
          setMyProjectsError("");
          return;
        }
        if (!s.profile) {
          setMyProjects(null);
          setMyProjectsError("");
        } else {
          ListMyProjectKeys()
            .then((list) => {
              if (!current()) return;
              setMyProjects(list ?? []);
              setMyProjectsError("");
            })
            .catch((e) => {
              if (!current()) return;
              setMyProjects(null);
              setMyProjectsError(errorMessage(e));
            });
        }
        ListProjects()
          .then((list) => {
            if (!current()) return;
            setProjects(list ?? []);
            setProjectsError("");
          })
          .catch((e) => {
            if (!current()) return;
            setProjects([]);
            setProjectsError(errorMessage(e));
          });
        ListProfiles()
          .then((list) => {
            if (!current()) return;
            setProfiles(list ?? []);
            setProfilesError("");
          })
          .catch((e) => {
            if (!current()) return;
            setProfiles([]);
            setProfilesError(errorMessage(e));
          });
      })
      .catch((e) => {
        if (!current()) return;
        setSaved(null);
        setSettingsError(errorMessage(e));
        setProjects([]);
        setProjectsError("");
        setProfiles([]);
        setProfilesError("");
        setMyProjects(null);
        setMyProjectsError("");
      });
  }, []);

  useEffect(() => {
    reload();
    return () => {
      request.current = -1;
    };
  }, [reload]);

  const open = useCallback((path: string) => setOpenRequest({ path, seq: ++seq.current }), []);

  const onDeleted = useCallback(() => {
    reload();
    setFilesRefresh((n) => n + 1);
  }, [reload]);

  return (
    <div className="nt-settings">
      <h1>Settings</h1>
      <div className="nt-tabs" role="tablist">
        <button className="nt-tabs__tab nt-tabs__tab--selected" role="tab" aria-selected="true" id="nt-tab-instructions" aria-controls="nt-panel-instructions">
          Instructions
        </button>
      </div>
      <div role="tabpanel" id="nt-panel-instructions" aria-labelledby="nt-tab-instructions">
        {saved && (saved.checkoutPath === "" || saved.profile === "") && <WelcomeCard saved={saved} />}
        <SetupForm saved={saved} settingsError={settingsError} projects={projects} projectsError={projectsError} profiles={profiles} profilesError={profilesError} myProjects={myProjects} myProjectsError={myProjectsError} onDeleted={onDeleted} onSaved={reload} onOpen={open} />
        <InstructionFiles onDirtyPath={onDirtyPath} archivePath={saved?.checkoutPath ?? ""} openRequest={openRequest} refresh={filesRefresh} />
        <PromptPreview projects={projects} projectsError={projectsError} />
        <UpdateCard />
      </div>
    </div>
  );
}
