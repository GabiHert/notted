import { useCallback, useEffect, useRef, useState } from "react";
import { EventsOn, GetReviewState, ReviewSuggestions } from "./api/bindings";
import { errorMessage } from "./format";

export type ReviewPhase = "idle" | "running" | "done" | "error";

// The suggestion review of one meeting: its phase follows GetReviewState and review:state.
export function useReview(meetingId: string): { phase: ReviewPhase; message: string; start: () => void } {
  const [state, setState] = useState<{ phase: ReviewPhase; message: string }>({ phase: "idle", message: "" });
  const idRef = useRef(meetingId);
  idRef.current = meetingId;

  useEffect(() => {
    let live = true;
    setState({ phase: "idle", message: "" });
    GetReviewState(meetingId)
      .then((s) => live && s?.running && setState((cur) => (cur.phase === "idle" ? { phase: "running", message: "" } : cur)))
      .catch(() => {});
    const off = EventsOn("review:state", (e: { meetingId: string; state: string; message?: string }) => {
      if (!live || e.meetingId !== meetingId) return;
      if (e.state === "running") setState({ phase: "running", message: "" });
      else if (e.state === "done") setState({ phase: "done", message: e.message ?? "" });
      else if (e.state === "error") setState({ phase: "error", message: e.message ?? "" });
    });
    return () => {
      live = false;
      off();
    };
  }, [meetingId]);

  const start = useCallback(() => {
    const id = meetingId;
    setState({ phase: "running", message: "" });
    ReviewSuggestions(id).catch((e) => {
      if (idRef.current === id) setState({ phase: "error", message: errorMessage(e) });
    });
  }, [meetingId]);

  return { phase: state.phase, message: state.message, start };
}
