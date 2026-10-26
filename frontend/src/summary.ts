import { useEffect, useRef, useState } from "react";
import { EventsOn } from "./api/bindings";

export type SummaryPass = "summary" | "suggestions";
export type SummaryRun = { pass: SummaryPass | null; error: string; setError: (message: string) => void };
type SummaryStateEvent = { meetingId: string; pass: SummaryPass; state: "running" | "done" | "error"; message?: string };

// The summary passes of one meeting, from summary:state: the page's only listener for that event.
// The Summary card shows the pass and its error; the Suggestions card blocks Review suggestions while a pass runs.
// No binding reports a pass that is already running, so one that started before this meeting was shown reads as idle until its next event.
export function useSummaryRun(meetingId: string, onDone: () => void): SummaryRun {
  const [pass, setPass] = useState<SummaryPass | null>(null);
  const [error, setError] = useState("");
  const doneRef = useRef(onDone);
  doneRef.current = onDone;

  useEffect(() => {
    setPass(null);
    setError("");
    return EventsOn("summary:state", (e: SummaryStateEvent) => {
      if (e.meetingId !== meetingId) return;
      if (e.state === "running") {
        setError("");
        setPass(e.pass);
      } else if (e.state === "error") {
        setPass(null);
        setError(e.message ?? "");
      } else {
        setPass(null);
        doneRef.current();
      }
    });
  }, [meetingId]);

  return { pass, error, setError };
}
