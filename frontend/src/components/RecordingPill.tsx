import { useEffect, useState } from "react";
import { elapsed } from "../format";
import "./RecordingPill.css";

export function RecordingPill({ startedAt }: { startedAt: string }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    setNow(Date.now());
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [startedAt]);
  return (
    <span className="nt-pill">
      <span className="nt-pill__dot" aria-hidden="true" />
      {`Recording ${elapsed(now - new Date(startedAt).getTime())}`}
    </span>
  );
}
