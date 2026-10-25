import { useState } from "react";
import { StartRecording, type wails } from "../api/bindings";
import { errorMessage } from "../format";
import { ModelBanner } from "./ModelBanner";
import "./NewRecording.css";

export function NewRecording({ onStarted }: { onStarted: (m: wails.MeetingDTO) => void }) {
  const [title, setTitle] = useState("");
  const [language, setLanguage] = useState("pt-BR");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setPending(true);
    setError("");
    try {
      const m = await StartRecording(title, language);
      onStarted(m);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="nt-newrec">
      <h1>New recording</h1>
      <ModelBanner language={language} />
      {error && <div className="nt-card nt-card--error">{error}</div>}
      <form className="nt-form" onSubmit={submit}>
        <label className="nt-field">
          <span>Title</span>
          <input value={title} placeholder="Meeting title (optional)" onChange={(e) => setTitle(e.target.value)} />
        </label>
        <label className="nt-field">
          <span>Language</span>
          <select value={language} onChange={(e) => setLanguage(e.target.value)}>
            <option value="pt-BR">Portuguese (Brazil)</option>
            <option value="en">English</option>
          </select>
        </label>
        <div>
          <button type="submit" className="nt-btn nt-btn--primary" disabled={pending}>
            {pending ? "Starting…" : "Start recording"}
          </button>
        </div>
      </form>
    </div>
  );
}
