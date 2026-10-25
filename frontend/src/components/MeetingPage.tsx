import { useCallback, useEffect, useRef, useState } from "react";
import { DeleteMeeting, EventsOn, GetDeletePlan, GetMeeting, GetTranscript, StopRecording, type wails } from "../api/bindings";
import { errorMessage, formatDate, languageName } from "../format";
import { ConfirmDialog } from "./ConfirmDialog";
import { ProjectCard } from "./ProjectCard";
import { ModelBanner } from "./ModelBanner";
import { RecordingPill } from "./RecordingPill";
import { SessionCard } from "./SessionCard";
import { SaveDialog } from "./SaveDialog";
import { SuggestionsCard } from "./SuggestionsCard";
import { SummaryCard } from "./SummaryCard";
import { TranscriptCard } from "./TranscriptCard";
import "./MeetingPage.css";

// onDeleted runs once the meeting is gone; the parent then leaves the page. The warning is "" unless something (the audio folder) was left behind.
export function MeetingPage({ meetingId, onDeleted }: { meetingId: string; onDeleted?: (warning: string) => void }) {
  const [meeting, setMeeting] = useState<wails.MeetingDTO | null>(null);
  const [segments, setSegments] = useState<wails.SegmentDTO[]>([]);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [stopping, setStopping] = useState(false);
  const [notCommitted, setNotCommitted] = useState(false);
  const [plan, setPlan] = useState<wails.DeletePlanDTO | null>(null);
  const deletedRef = useRef(false); // set once the meeting is gone: later events for it are ignored
  const loadRef = useRef<() => void>(() => {});
  const refresh = useCallback(() => loadRef.current(), []);

  useEffect(() => {
    let live = true;
    setMeeting(null);
    setSegments([]);
    setWarnings([]);
    setError("");
    setStopping(false);
    setNotCommitted(false);
    setPlan(null);
    deletedRef.current = false;
    const load = () =>
      deletedRef.current ? Promise.resolve() :
      GetMeeting(meetingId)
        .then((m) => live && setMeeting(m))
        .catch((e) => live && setError(errorMessage(e)));
    loadRef.current = load;
    load();
    GetTranscript(meetingId)
      .then((list) => {
        if (!live) return;
        setSegments((cur) => mergeSegments(list ?? [], cur));
      })
      .catch(() => {});
    const offState = EventsOn("recording:state", (e: { meetingId: string; warning?: string; error?: string }) => {
      if (e.meetingId !== meetingId || deletedRef.current) return;
      if (e.warning) setWarnings((cur) => (cur.includes(e.warning!) ? cur : [...cur, e.warning!]));
      if (e.error) setError(e.error);
      load();
    });
    const offSeg = EventsOn("transcript:segment", (e: { meetingId: string; segment: wails.SegmentDTO }) => {
      if (e.meetingId !== meetingId || deletedRef.current) return;
      setSegments((cur) => mergeSegments(cur, [e.segment]));
    });
    const offExport = EventsOn("export:ready", (e: { meetingId: string }) => {
      if (e.meetingId === meetingId) setNotCommitted(false);
    });
    return () => {
      live = false;
      offState();
      offSeg();
      offExport();
    };
  }, [meetingId]);

  if (!meeting) return error ? <div className="nt-card nt-card--error" role="alert">
      {error}
    </div> : null;

  const stop = () => {
    setStopping(true);
    StopRecording(meetingId).catch((e) => {
      setStopping(false);
      setError(errorMessage(e));
    });
  };

  const askDelete = () => {
    setError("");
    GetDeletePlan(meetingId)
      .then(setPlan)
      .catch((e) => setError(errorMessage(e)));
  };
  const confirmDelete = () => {
    setPlan(null);
    DeleteMeeting(meetingId)
      .then((r) => {
        deletedRef.current = true;
        onDeleted?.(r?.warning ?? "");
      })
      .catch((e) => setError(errorMessage(e)));
  };

  return (
    <div className="nt-meeting">
      <header className="nt-meeting__head">
        <div>
          <h1>{meeting.title}</h1>
          <div className="nt-meeting__meta">{`${formatDate(meeting.startedAt)} · ${languageName(meeting.language)}`}</div>
        </div>
        <div className="nt-meeting__live">
          {meeting.state === "recording" && (
            <>
              <RecordingPill startedAt={meeting.startedAt} />
              <button className="nt-btn nt-btn--primary" onClick={stop} disabled={stopping}>
                Stop recording
              </button>
            </>
          )}
          <button
            className="nt-btn nt-btn--danger-quiet"
            onClick={askDelete}
            disabled={meeting.state === "recording"}
            title={meeting.state === "recording" ? "Stop the recording before you delete this meeting." : undefined}
          >
            Delete meeting
          </button>
        </div>
      </header>
      {meeting.state === "transcribing" && (
        <div className="nt-meeting__busy">
          <span>Transcribing…</span>
          <div className="nt-bar nt-bar--indeterminate">
            <div className="nt-bar__fill" />
          </div>
        </div>
      )}
      <ModelBanner language={meeting.language} />
      {warnings.length > 0 && (
        <section className="nt-card">
          <div className="nt-card__header">Audio</div>
          {warnings.map((w) => (
            <p key={w} className="nt-meeting__msg">
              {w}
            </p>
          ))}
        </section>
      )}
      {error && <div className="nt-card nt-card--error" role="alert">
      {error}
    </div>}
      <TranscriptCard segments={segments} follow={meeting.state === "recording"} />
      <ProjectCard meetingId={meetingId} projectKey={meeting.projectKey} hasSession={meeting.hasSession} onChanged={refresh} />
      <SummaryCard meeting={meeting} notCommitted={notCommitted} onRefresh={refresh} />
      <SuggestionsCard suggestionsMd={meeting.suggestionsMd} />
      <SessionCard key={meetingId} meetingId={meetingId} hasSession={meeting.hasSession} />
      <SaveDialog meetingId={meetingId} onDeclined={() => setNotCommitted(true)} />
      {plan && (
        <ConfirmDialog
          title={`Delete “${meeting.title}”?`}
          body={`The audio (${plan.audioFiles} ${plan.audioFiles === 1 ? "file" : "files"}), the transcript, the summary and the suggestions.`}
          hint={"This cannot be undone." + (plan.archiveFolder ? ` The meeting's folder in the archive (${plan.archiveFolder}) is not changed.` : "")}
          cancelLabel="Cancel"
          confirmLabel="Delete meeting"
          destructive
          onCancel={() => setPlan(null)}
          onConfirm={confirmDelete}
        />
      )}
    </div>
  );
}

function mergeSegments(a: wails.SegmentDTO[], b: wails.SegmentDTO[]): wails.SegmentDTO[] {
  const seen = new Set(a.map((s) => s.id));
  const out = [...a];
  for (const s of b) {
    if (!seen.has(s.id)) {
      seen.add(s.id);
      out.push(s);
    }
  }
  return out;
}
