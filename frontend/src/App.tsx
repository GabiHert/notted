import { useEffect, useState } from "react";
import { ListMeetings } from "./api/bindings";
import { MeetingPage } from "./components/MeetingPage";
import { NewRecording } from "./components/NewRecording";
import { Sidebar, type View } from "./components/Sidebar";
import { TitleBar } from "./components/TitleBar";
import "./App.css";

export function App({ initialView }: { initialView?: View }) {
  const [view, setView] = useState<View | null>(initialView ?? null);

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
  return (
    <div className="nt-window">
      <TitleBar />
      <div className="nt-body">
        <Sidebar view={current} onSelect={setView} />
        <main className="nt-main">
          <div className="nt-main__col">
            {view === null ? null : view.kind === "new" ? (
              <NewRecording onStarted={(m) => setView({ kind: "meeting", id: m.id })} />
            ) : view.kind === "meeting" ? (
              <MeetingPage meetingId={view.id} />
            ) : (
              <h1>Settings</h1>
            )}
          </div>
        </main>
      </div>
    </div>
  );
}
