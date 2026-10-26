import type { wails } from "../api/bindings";
import { HtmlFrame } from "./HtmlFrame";
import { MarkdownView } from "./MarkdownView";

export type Answer = string | string[];

export function FormPiece({
  piece,
  value,
  disabled,
  onChange,
}: {
  piece: wails.FormPieceDTO;
  value: Answer | undefined;
  disabled: boolean;
  onChange: (v: Answer) => void;
}) {
  switch (piece.kind) {
    case "text":
      return <MarkdownView content={piece.markdown} untrusted />;
    case "html":
      return <HtmlFrame html={piece.html} />;
    case "choice":
    case "multi": {
      const multi = piece.kind === "multi";
      const picked = multi ? ((value as string[] | undefined) ?? []) : [];
      return (
        <fieldset className="nt-form__field" disabled={disabled}>
          <legend className="nt-form__question">{piece.question}</legend>
          {piece.options.map((o) => (
            <label key={o.label} className="nt-form__option">
              <input
                type={multi ? "checkbox" : "radio"}
                name={"nt-" + piece.id}
                checked={multi ? picked.includes(o.label) : value === o.label}
                onChange={() =>
                  onChange(multi ? piece.options.map((x) => x.label).filter((l) => (l === o.label ? !picked.includes(l) : picked.includes(l))) : o.label)
                }
              />
              <span className="nt-form__label">
                {o.label}
                {o.recommended && <span className="nt-form__tag">Recommended</span>}
              </span>
              {o.description !== "" && <span className="nt-form__desc">{o.description}</span>}
            </label>
          ))}
        </fieldset>
      );
    }
    case "input": {
      const id = "nt-form-" + piece.id;
      const text = (value as string | undefined) ?? "";
      return (
        <>
          <label className="nt-form__question" htmlFor={id}>
            {piece.question}
            {piece.required ? " (required)" : ""}
          </label>
          {piece.multiline ? (
            <textarea id={id} rows={4} placeholder={piece.placeholder} value={text} disabled={disabled} onChange={(e) => onChange(e.target.value)} />
          ) : (
            <input id={id} type="text" placeholder={piece.placeholder} value={text} disabled={disabled} onChange={(e) => onChange(e.target.value)} />
          )}
        </>
      );
    }
    case "confirm":
      return (
        <fieldset className="nt-form__field" disabled={disabled}>
          <legend className="nt-form__question">{piece.question}</legend>
          <div className="nt-form__confirm">
            {(
              [
                ["yes", piece.yesLabel],
                ["no", piece.noLabel],
              ] as const
            ).map(([v, label]) => (
              <button key={v} type="button" className={value === v ? "nt-btn nt-btn--primary" : "nt-btn"} aria-pressed={value === v} onClick={() => onChange(v)}>
                {label}
              </button>
            ))}
          </div>
        </fieldset>
      );
    default:
      return null;
  }
}
