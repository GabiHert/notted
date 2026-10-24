import { useEffect, useState } from "react";
import { EventsOn, GetMeeting, GetTranscript, StopRecording, type wails } from "../api/bindings";
import { errorMessage, formatDate, languageName } from "../format";
import { ModelBanner } from "./ModelBanner";
import { RecordingPill } from "./RecordingPill";
import { TranscriptCard } from "./TranscriptCard";
import "./MeetingPage.css";

export function MeetingPage({ meetingId }: { meetingId: string }) {
  const [meeting, setMeeting] = useState<wails.MeetingDTO | null>(null);
  const [segments, setSegments] = useState<wails.SegmentDTO[]>([]);
  const [warning, setWarning] = useState("");
  const [error, setError] = useState("");
  const [stopping, setStopping] = useState(false);

  useEffect(() => {
    let live = true;
    setMeeting(null);
    setSegments([]);
    setWarning("");
    setError("");
    setStopping(false);
    const load = () =>
      GetMeeting(meetingId)
        .then((m) => live && setMeeting(m))
        .catch((e) => live && setError(errorMessage(e)));
    load();
    GetTranscript(meetingId)
      .then((list) => {
        if (!live) return;
        setSegments((cur) => mergeSegments(list ?? [], cur));
      })
      .catch(() => {});
    const offState = EventsOn("recording:state", (e: { meetingId: string; warning?: string; error?: string }) => {
      if (e.meetingId !== meetingId) return;
      if (e.warning) setWarning(e.warning);
      if (e.error) setError(e.error);
      load();
    });
    const offSeg = EventsOn("transcript:segment", (e: { meetingId: string; segment: wails.SegmentDTO }) => {
      if (e.meetingId !== meetingId) return;
      setSegments((cur) => mergeSegments(cur, [e.segment]));
    });
    return () => {
      live = false;
      offState();
      offSeg();
    };
  }, [meetingId]);

  if (!meeting) return error ? <div className="nt-card nt-card--error">{error}</div> : null;

  const stop = () => {
    setStopping(true);
    StopRecording(meetingId).catch((e) => {
      setStopping(false);
      setError(errorMessage(e));
    });
  };

  return (
    <div className="nt-meeting">
      <header className="nt-meeting__head">
        <div>
          <h1>{meeting.title}</h1>
          <div className="nt-meeting__meta">{`${formatDate(meeting.startedAt)} · ${languageName(meeting.language)}`}</div>
        </div>
        {meeting.state === "recording" && (
          <div className="nt-meeting__live">
            <RecordingPill startedAt={meeting.startedAt} />
            <button className="nt-btn nt-btn--primary" onClick={stop} disabled={stopping}>
              Stop recording
            </button>
          </div>
        )}
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
      {warning && (
        <section className="nt-card">
          <div className="nt-card__header">System audio</div>
          <p className="nt-meeting__msg">{warning}</p>
        </section>
      )}
      {error && <div className="nt-card nt-card--error">{error}</div>}
      <TranscriptCard segments={segments} follow={meeting.state === "recording"} />
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
