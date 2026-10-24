import { useCallback, useEffect, useRef, useState } from "react";
import { ListProjects } from "../api/bindings";
import { errorMessage } from "../format";
import { InstructionFiles } from "./InstructionFiles";
import { PromptPreview } from "./PromptPreview";
import { SetupForm } from "./SetupForm";
import "./SettingsView.css";

export function SettingsView({ onDirtyPath }: { onDirtyPath?: (path: string | null) => void } = {}) {
  const [projects, setProjects] = useState<string[]>([]);
  const [projectsError, setProjectsError] = useState("");
  const request = useRef(0);

  const loadProjects = useCallback(() => {
    const id = ++request.current;
    ListProjects()
      .then((list) => {
        if (request.current !== id) return;
        setProjects(list ?? []);
        setProjectsError("");
      })
      .catch((e) => {
        if (request.current !== id) return;
        setProjects([]);
        setProjectsError(errorMessage(e));
      });
  }, []);

  useEffect(() => {
    loadProjects();
    return () => {
      request.current = -1;
    };
  }, [loadProjects]);

  return (
    <div className="nt-settings">
      <h1>Settings</h1>
      <div className="nt-tabs" role="tablist">
        <button className="nt-tabs__tab nt-tabs__tab--selected" role="tab" aria-selected="true" id="nt-tab-instructions" aria-controls="nt-panel-instructions">
          Instructions
        </button>
      </div>
      <div role="tabpanel" id="nt-panel-instructions" aria-labelledby="nt-tab-instructions">
        <SetupForm projects={projects} projectsError={projectsError} onSaved={loadProjects} />
        <InstructionFiles onDirtyPath={onDirtyPath} />
        <PromptPreview projects={projects} projectsError={projectsError} />
      </div>
    </div>
  );
}
