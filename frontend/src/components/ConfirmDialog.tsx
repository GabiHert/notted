import { useEffect, useRef } from "react";
import "./SaveDialog.css";

type Props = { title: string; body: string; items?: string[]; hint?: string; cancelLabel: string; confirmLabel: string; destructive?: boolean; onCancel: () => void; onConfirm: () => void };

// A two-button modal (destructive paints the confirm button red) in the same style as the save dialog. Escape means the cancel button.
// Focus stays inside while it is open; the key handler sits on the document and is removed on close.
export function ConfirmDialog({ title, body, items, hint, cancelLabel, confirmLabel, destructive, onCancel, onConfirm }: Props) {
  const panel = useRef<HTMLDivElement>(null);
  const cancel = useRef(onCancel);
  cancel.current = onCancel;

  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    panel.current?.querySelector<HTMLElement>("button")?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        cancel.current();
        return;
      }
      if (e.key !== "Tab") return;
      const items = [...(panel.current?.querySelectorAll<HTMLElement>("button:not(:disabled)") ?? [])];
      if (items.length === 0) {
        e.preventDefault();
        panel.current?.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      const inside = panel.current?.contains(active) ?? false;
      if (e.shiftKey && (active === first || !inside)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (active === last || !inside)) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      if (before && document.contains(before)) before.focus();
    };
  }, []);

  return (
    <div className="nt-overlay">
      <div className="nt-dialog" role="dialog" aria-modal="true" aria-labelledby="nt-confirm-title" ref={panel} tabIndex={-1}>
        <h3 id="nt-confirm-title">{title}</h3>
        <p className="nt-dialog__text">{body}</p>
        {items && items.length > 0 && (
          <ul className="nt-dialog__files">
            {items.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        )}
        {hint && <p className="nt-dialog__hint">{hint}</p>}
        <div className="nt-dialog__actions">
          <button className="nt-btn" onClick={onCancel}>
            {cancelLabel}
          </button>
          <button className={"nt-btn " + (destructive ? "nt-btn--danger" : "nt-btn--primary")} onClick={onConfirm}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
