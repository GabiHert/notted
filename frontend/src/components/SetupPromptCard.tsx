import { useEffect, useRef, useState } from "react";
import { ClipboardSetText, GetSetupPrompt } from "../api/bindings";
import { errorMessage } from "../format";
import "./SetupPromptCard.css";

const NOTE = "Paste a prompt into Claude Code and it will explain how Notted works, then ask you the questions that fill in your profile and project files.";
const COPIED = "Copied. Paste it into Claude Code.";
const FAILED = "Could not copy.";
const COPIED_MS = 4000;

// The card copies the setup prompt to the clipboard. Show prompt reveals the same text, read-only, so it can be read first
// or selected by hand when the clipboard is unavailable. Every answer is checked against the newest request.
export function SetupPromptCard() {
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [text, setText] = useState<string | null>(null);
  const [shown, setShown] = useState(false);
  const request = useRef(0);
  const showRequest = useRef(0);
  const timer = useRef<number | undefined>(undefined);

  useEffect(
    () => () => {
      request.current = -1;
      showRequest.current = -1;
      window.clearTimeout(timer.current);
    },
    [],
  );

  const say = (s: string, ms = 0) => {
    window.clearTimeout(timer.current);
    setStatus(s);
    if (ms) timer.current = window.setTimeout(() => setStatus(""), ms);
  };

  const copy = () => {
    const id = ++request.current;
    setError("");
    say("");
    GetSetupPrompt()
      .then((t) => {
        if (request.current !== id) return;
        setText(t);
        return ClipboardSetText(t).then((ok) => {
          if (request.current !== id) return;
          if (ok) say(COPIED, COPIED_MS);
          else fail();
        }, () => {
          if (request.current === id) fail();
        });
      })
      .catch((e) => {
        if (request.current !== id) return;
        setError(errorMessage(e));
        say(FAILED);
      });
  };

  const fail = () => {
    setShown(true);
    say(FAILED);
  };

  // Show prompt has its own counter, so it never cancels a Copy, and it reads the prompt each time it opens:
  // the archive or the profile may have changed since the last time.
  const toggle = () => {
    const id = ++showRequest.current;
    if (shown) {
      setShown(false);
      return;
    }
    setError("");
    setText(null);
    setShown(true);
    GetSetupPrompt()
      .then((t) => {
        if (showRequest.current === id) setText(t);
      })
      .catch((e) => {
        if (showRequest.current !== id) return;
        setShown(false);
        setError(errorMessage(e));
      });
  };

  return (
    <section className="nt-card nt-setup-prompt">
      <div className="nt-card__header">Set up with Claude Code</div>
      <p className="nt-setup-prompt__note">{NOTE}</p>
      <div className="nt-setup-prompt__row">
        <button type="button" className="nt-btn nt-btn--primary" onClick={copy}>
          Copy setup prompt
        </button>
        <button type="button" className="nt-btn" onClick={toggle} aria-expanded={shown}>
          {shown ? "Hide prompt" : "Show prompt"}
        </button>
        <span className="nt-setup-prompt__status" role="status">
          {status}
        </span>
      </div>
      {error && (
        <div className="nt-card nt-card--error nt-setup-prompt__error" role="alert">
          {error}
        </div>
      )}
      {shown && text !== null && (
        <textarea className="nt-setup-prompt__out" readOnly aria-label="Setup prompt" data-testid="setup-prompt-output" value={text} onFocus={(e) => e.currentTarget.select()} />
      )}
    </section>
  );
}
