import { useEffect, useRef, useState } from "react";
import { EventsOn, GetSettings, ListMeetings, SetWindowVisible } from "./api/bindings";
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
  const [menuError, setMenuError] = useState("");

  // A start or stop from the menu bar: show its failure, and follow a start into its meeting. The sidebar refreshes the list itself.
  useEffect(() => {
    const offError = EventsOn("menubar:error", (e: { message: string }) => setMenuError(e.message));
    const offState = EventsOn("recording:state", (e: { meetingId: string; state: string; warning?: string }) => {
      if (e.state !== "recording" || e.warning) return; // a silence warning is not a start
      setMenuError("");
      setView((v) => (v !== null && v.kind === "new" ? { kind: "meeting", id: e.meetingId } : v));
    });
    return () => {
      offError();
      offState();
    };
  }, []);

  // Tell the backend whether the window is visible, so it notifies only when the person is away.
  useEffect(() => {
    const report = () => void SetWindowVisible(document.visibilityState === "visible").catch(() => {});
    report();
    document.addEventListener("visibilitychange", report);
    return () => document.removeEventListener("visibilitychange", report);
  }, []);

  useEffect(() => {
    if (initialView) return;
    let live = true;
    // First run: with no archive folder saved, Settings opens first, even when meetings exist.
    Promise.all([GetSettings().catch(() => null), ListMeetings().catch(() => null)]).then(([settings, list]) => {
      if (!live) return;
      if (settings && !settings.checkoutPath) setView({ kind: "settings" });
      else setView(list && list.length > 0 ? { kind: "meeting", id: list[0].id } : { kind: "new" });
    });
    return () => {
      live = false;
    };
  }, [initialView]);

  const current: View = view ?? { kind: "new" };
  // Leaving Settings unmounts the Instructions tab, so ask first when the open file has unsaved changes.
  const navigate = (v: View) => {
    setMenuError("");
    if (current.kind === "settings" && v.kind !== "settings" && dirtyPath) setPendingView(v);
    else setView(v);
  };
  // A click on a form notification: the backend has shown the window; open that meeting like a sidebar click.
  const navigateRef = useRef(navigate);
  navigateRef.current = navigate;
  useEffect(() => EventsOn("ask:open", (e: { meetingId: string }) => navigateRef.current({ kind: "meeting", id: e.meetingId })), []);
  // After a delete: the newest meeting left, or New recording when none is.
  const leaveDeleted = (warning: string) => {
    setMenuError(warning);
    ListMeetings()
      .catch(() => null)
      .then((list) => setView(list && list.length > 0 ? { kind: "meeting", id: list[0].id } : { kind: "new" }));
  };
  return (
    <div className="nt-window">
      <TitleBar />
      <div className="nt-body">
        <Sidebar view={current} onSelect={navigate} />
        <main className="nt-main">
          <div className={"nt-main__col" + (current.kind === "settings" ? " nt-main__col--wide" : "")}>
            {menuError && (
              <div className="nt-card nt-card--error" role="alert">
                {menuError}
              </div>
            )}
            {view === null ? null : view.kind === "new" ? (
              <NewRecording
                onStarted={(m) => {
                  setMenuError("");
                  setView({ kind: "meeting", id: m.id });
                }}
              />
            ) : view.kind === "meeting" ? (
              <MeetingPage meetingId={view.id} onDeleted={leaveDeleted} />
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
