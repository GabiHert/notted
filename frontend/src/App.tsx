import { useEffect, useState } from "react";
import { ListMeetings } from "./api/bindings";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { MeetingPage } from "./components/MeetingPage";
import { NewRecording } from "./components/NewRecording";
import { SettingsView } from "./components/SettingsView";
import { Sidebar, type View } from "./components/Sidebar";
import { TitleBar } from "./components/TitleBar";
import "./App.css";

export function App({ initialView }: { initialView?: View }) {
  const [view, setView] = useState<View | null>(initialView ?? null);
  const [dirtyPath, setDirtyPath] = useState<string | null>(null);
  const [pendingView, setPendingView] = useState<View | null>(null);

  useEffect(() => {
    if (initialView) return;
    let live = true;
    ListMeetings()
      .then((list) => live && setView(list && list.length > 0 ? { kind: "meeting", id: list[0].id } : { kind: "new" }))
      .catch(() => live && setView({ kind: "new" }));
    return () => {
      live = false;
    };
  }, [initialView]);

  const current: View = view ?? { kind: "new" };
  // Leaving Settings unmounts the Instructions tab, so ask first when the open file has unsaved changes.
  const navigate = (v: View) => {
    if (current.kind === "settings" && v.kind !== "settings" && dirtyPath) setPendingView(v);
    else setView(v);
  };
  return (
    <div className="nt-window">
      <TitleBar />
      <div className="nt-body">
        <Sidebar view={current} onSelect={navigate} />
        <main className="nt-main">
          <div className={"nt-main__col" + (current.kind === "settings" ? " nt-main__col--wide" : "")}>
            {view === null ? null : view.kind === "new" ? (
              <NewRecording onStarted={(m) => setView({ kind: "meeting", id: m.id })} />
            ) : view.kind === "meeting" ? (
              <MeetingPage meetingId={view.id} />
            ) : (
              <SettingsView onDirtyPath={setDirtyPath} />
            )}
          </div>
        </main>
      </div>
      {pendingView && (
        <ConfirmDialog
          title="Discard unsaved changes?"
          body={`Your changes to ${dirtyPath ?? ""} are not saved.`}
          cancelLabel="Keep editing"
          confirmLabel="Discard"
          onCancel={() => setPendingView(null)}
          onConfirm={() => {
            setPendingView(null);
            setView(pendingView);
          }}
        />
      )}
    </div>
  );
}
