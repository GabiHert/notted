import { useEffect, useState } from "react";
import { ListProjects, SetMeetingProject } from "../api/bindings";
import { errorMessage } from "../format";
import "./ProjectCard.css";

type Props = { meetingId: string; projectKey: string; hasSession: boolean; onChanged: () => void };

export function ProjectCard({ meetingId, projectKey, hasSession, onChanged }: Props) {
  const [projects, setProjects] = useState<string[] | null>(null);
  const [listError, setListError] = useState("");
  const [value, setValue] = useState(projectKey);
  const [error, setError] = useState("");

  useEffect(() => setValue(projectKey), [projectKey]);

  useEffect(() => {
    let live = true;
    setProjects(null);
    setListError("");
    ListProjects()
      .then((list) => live && setProjects(list ?? []))
      .catch((e) => live && setListError(errorMessage(e)));
    return () => {
      live = false;
    };
  }, [meetingId]);

  const change = (key: string) => {
    const previous = value;
    setValue(key);
    setError("");
    SetMeetingProject(meetingId, key)
      .then(onChanged)
      .catch((e) => {
        setValue(previous);
        setError(errorMessage(e));
      });
  };

  const options = projects ?? [];
  const shown = value && !options.includes(value) ? [...options, value] : options;

  return (
    <section className="nt-card">
      <div className="nt-card__header">Project</div>
      {listError ? (
        <p className="nt-project__note">{listError}</p>
      ) : projects !== null && projects.length === 0 ? (
        <p className="nt-project__note">No project files found in instructions/projects/ of the archive folder.</p>
      ) : (
        <label className="nt-project__label">
          <span>Project for this meeting</span>
          <select className="nt-select" value={value} disabled={hasSession} onChange={(e) => change(e.target.value)}>
            <option value="">Choose a project</option>
            {shown.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </select>
        </label>
      )}
      {hasSession && <p className="nt-project__note">The project is fixed once the summary exists.</p>}
      {error && (
        <div className="nt-card nt-card--error nt-project__error" role="alert">
          {error}
        </div>
      )}
    </section>
  );
}
