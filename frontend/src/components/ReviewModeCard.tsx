import { useEffect, useRef, useState } from "react";
import { GetReviewActWithoutAsking, SetReviewActWithoutAsking } from "../api/bindings";
import { errorMessage } from "../format";
import "./ReviewModeCard.css";

const LABEL = "Let Claude act without asking during reviews";
const HELP = "When this is on, Claude edits files and runs commands, git included, during Review suggestions without asking you first. You still answer the suggestions form. The Claude Code session on a meeting's page still asks in the terminal. A change applies from the next review.";

// The card loads its own state. The box shows a new value only after the save succeeds, and results that
// arrive after unmount are dropped.
export function ReviewModeCard() {
  const [on, setOn] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const live = useRef(true);

  useEffect(() => {
    live.current = true;
    GetReviewActWithoutAsking()
      .then((v) => {
        if (!live.current) return;
        setOn(v);
        setLoaded(true);
      })
      .catch((e) => {
        if (live.current) setError(errorMessage(e));
      });
    return () => {
      live.current = false;
    };
  }, []);

  const toggle = async () => {
    if (busy) return;
    const next = !on;
    setBusy(true);
    setError("");
    try {
      await SetReviewActWithoutAsking(next);
      if (live.current) setOn(next);
    } catch (e) {
      if (live.current) setError(errorMessage(e));
    } finally {
      if (live.current) setBusy(false);
    }
  };

  return (
    <section className="nt-card nt-reviewmode">
      <div className="nt-card__header">Suggestion reviews</div>
      <div className="nt-reviewmode__option">
        <input id="nt-reviewmode" type="checkbox" checked={on} disabled={!loaded || busy} onChange={toggle} aria-describedby="nt-reviewmode-help" />
        <label htmlFor="nt-reviewmode" className="nt-reviewmode__label">{LABEL}</label>
        <p id="nt-reviewmode-help" className="nt-reviewmode__desc">{HELP}</p>
      </div>
      {error && <div className="nt-card nt-card--error nt-reviewmode__error" role="alert">{error}</div>}
    </section>
  );
}
