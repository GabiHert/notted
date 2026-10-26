import { useState, type Ref } from "react";
import { AnswerForm, DismissForm, type wails } from "../api/bindings";
import { errorMessage } from "../format";
import { FormPiece, type Answer } from "./FormPiece";
import "./FormCard.css";

const answerable = (p: wails.FormPieceDTO) => p.kind === "choice" || p.kind === "multi" || p.kind === "input" || p.kind === "confirm";

export function FormCard({ form, sectionRef }: { form: wails.OpenFormDTO; sectionRef?: Ref<HTMLElement> }) {
  const [values, setValues] = useState<Record<string, Answer>>({});
  const [note, setNote] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const fields = form.pieces.filter(answerable);
  const confirmOnly = fields.length === 1 && fields[0].kind === "confirm";
  const ready = fields.every((p) => {
    const v = values[p.id];
    if (p.kind === "choice" || p.kind === "confirm") return typeof v === "string" && v !== "";
    if (p.kind === "input" && p.required) return typeof v === "string" && v.trim() !== "";
    return true;
  });

  const run = (call: Promise<void>) => {
    setPending(true);
    setError("");
    call.catch((e) => {
      setPending(false);
      setError(errorMessage(e));
    });
  };
  const answersWith = (extra: Record<string, Answer> = {}) => {
    const all = { ...values, ...extra };
    const out: Record<string, Answer> = {};
    for (const p of fields) out[p.id] = all[p.id] ?? (p.kind === "multi" ? [] : "");
    return out;
  };
  const send = () => run(AnswerForm(form.id, answersWith(), note));
  const change = (p: wails.FormPieceDTO, v: Answer) => {
    if (confirmOnly) run(AnswerForm(form.id, answersWith({ [p.id]: v }), note));
    else setValues((cur) => ({ ...cur, [p.id]: v }));
  };

  return (
    <section className="nt-card nt-formcard" aria-label="Waiting for you" tabIndex={-1} ref={sectionRef}>
      <div className="nt-card__header">{form.kind === "permission" ? "Waiting for you · Permission" : "Waiting for you"}</div>
      <h2 className="nt-form__title">{form.title}</h2>
      {form.queued > 0 && (
        <p className="nt-form__queue">{form.queued === 1 ? "1 more form is waiting after this one." : `${form.queued} more forms are waiting after this one.`}</p>
      )}
      {form.pieces.map((p) => (
        <div key={p.id} className="nt-form__piece">
          <FormPiece piece={p} value={values[p.id]} disabled={pending} onChange={(v) => change(p, v)} />
        </div>
      ))}
      <label className="nt-form__question" htmlFor="nt-form-note">
        Note (optional)
      </label>
      <textarea id="nt-form-note" rows={3} placeholder="Anything Claude should know" value={note} disabled={pending} onChange={(e) => setNote(e.target.value)} />
      <div className="nt-form__actions">
        {!confirmOnly && (
          <button className="nt-btn nt-btn--primary" onClick={send} disabled={pending || !ready}>
            Send
          </button>
        )}
        <button className="nt-btn" onClick={() => run(DismissForm(form.id, note))} disabled={pending}>
          Not now
        </button>
      </div>
      {error && (
        <div className="nt-card nt-card--error" role="alert">
          {error}
        </div>
      )}
    </section>
  );
}
