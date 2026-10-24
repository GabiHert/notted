import { useEffect, useRef, useState } from "react";
import { PreviewPrompt } from "../api/bindings";
import { errorMessage } from "../format";
import "./PromptPreview.css";

type Props = { clients: string[]; clientsError: string };

export function PromptPreview({ clients, clientsError }: Props) {
  const [key, setKey] = useState("");
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const request = useRef(0);

  useEffect(
    () => () => {
      request.current = -1;
    },
    [],
  );

  const choose = (k: string) => {
    request.current++;
    setKey(k);
    setText(null);
    setError("");
    setBusy(false);
  };

  const run = () => {
    const id = ++request.current;
    setBusy(true);
    setError("");
    PreviewPrompt(key)
      .then((t) => {
        if (request.current !== id) return;
        setText(t);
        setBusy(false);
      })
      .catch((e) => {
        if (request.current !== id) return;
        setText(null);
        setError(errorMessage(e));
        setBusy(false);
      });
  };

  return (
    <section className="nt-card">
      <div className="nt-card__header">Prompt preview</div>
      <p className="nt-preview__note">This is exactly what the next summary for this client receives.</p>
      {clientsError && <p className="nt-preview__note">{clientsError}</p>}
      <div className="nt-preview__row">
        <label htmlFor="nt-preview-client">Preview the prompt for</label>
        <select id="nt-preview-client" value={key} onChange={(e) => choose(e.target.value)}>
          <option value="">Choose a client</option>
          {clients.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
        <button className="nt-btn" onClick={run} disabled={!key || busy}>
          Preview prompt
        </button>
      </div>
      {error && (
        <div className="nt-card nt-card--error nt-preview__error" role="alert">
          {error}
        </div>
      )}
      {text !== null && (
        <pre className="nt-preview__out" data-testid="prompt-output">
          {text}
        </pre>
      )}
    </section>
  );
}
