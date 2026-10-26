import { useCallback, useEffect, useState } from "react";
import { EventsOn, ListMeetings, type wails } from "../api/bindings";
import { useOpenForms } from "../forms";
import { formatDate, languageName, stateName } from "../format";
import "./Sidebar.css";

export type View = { kind: "new" } | { kind: "meeting"; id: string; reveal?: number } | { kind: "settings" };

type Props = { view: View; onSelect: (v: View) => void };

export function Sidebar({ view, onSelect }: Props) {
  const [meetings, setMeetings] = useState<wails.MeetingDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const forms = useOpenForms();

  const refresh = useCallback(() => {
    ListMeetings()
      .then((list) => setMeetings(list ?? []))
      .catch(() => {})
      .finally(() => setLoaded(true));
  }, []);

  useEffect(() => {
    refresh();
    const offState = EventsOn("recording:state", refresh);
    const offDeleted = EventsOn("meeting:deleted", refresh);
    return () => {
      offState();
      offDeleted();
    };
  }, [refresh]);

  return (
    <nav className="nt-sidebar" aria-label="Meetings">
      <button className="nt-btn nt-btn--primary nt-sidebar__new" onClick={() => onSelect({ kind: "new" })}>
        New recording
      </button>
      <div className="nt-sidebar__list">
        {loaded && meetings.length === 0 && <p className="nt-sidebar__empty">No meetings yet.</p>}
        {meetings.map((m) => {
          const selected = view.kind === "meeting" && view.id === m.id;
          return (
            <button
              key={m.id}
              className={"nt-row" + (selected ? " nt-row--selected" : "")}
              aria-current={selected ? "true" : undefined}
              onClick={() => onSelect({ kind: "meeting", id: m.id })}
            >
              <span className="nt-row__title">
                {m.title}
                {forms.has(m.id) && <span className="nt-row__ask" role="img" aria-label="Waiting for you" title="Waiting for you" />}
              </span>
              <span className="nt-row__meta">
                <span className={"nt-dot" + (m.state === "recording" ? " nt-dot--rec" : "")} aria-hidden="true" />
                <span>{`${formatDate(m.startedAt)} · ${languageName(m.language)} · ${stateName(m.state)}`}</span>
              </span>
            </button>
          );
        })}
      </div>
      <button
        className={"nt-sidebar__settings" + (view.kind === "settings" ? " nt-row--selected" : "")}
        aria-current={view.kind === "settings" ? "true" : undefined}
        onClick={() => onSelect({ kind: "settings" })}
      >
        Settings
      </button>
    </nav>
  );
}
