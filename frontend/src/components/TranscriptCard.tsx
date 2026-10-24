import { useEffect, useRef } from "react";
import type { wails } from "../api/bindings";
import { clock, speakerName } from "../format";
import "./TranscriptCard.css";

export function TranscriptCard({ segments, follow }: { segments: wails.SegmentDTO[]; follow: boolean }) {
  const box = useRef<HTMLDivElement>(null);
  const stuck = useRef(true);

  useEffect(() => {
    const el = box.current;
    if (follow && stuck.current && el) el.scrollTop = el.scrollHeight;
  }, [segments, follow]);

  const onScroll = () => {
    const el = box.current;
    if (el) stuck.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  };

  return (
    <section className="nt-card">
      <div className="nt-card__header">Transcript</div>
      {segments.length === 0 ? (
        <p className="nt-transcript__empty">The transcript appears here as each 30-second part is transcribed.</p>
      ) : (
        <div className="nt-transcript" ref={box} onScroll={onScroll}>
          {segments.map((s) => (
            <div className="nt-line" key={s.id}>
              <span className="nt-line__time">{clock(s.startMs)}</span>
              <span className="nt-line__speaker">{speakerName(s.track)}</span>
              <span className="nt-line__text">{s.text}</span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
