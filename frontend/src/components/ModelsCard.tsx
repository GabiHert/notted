import { useCallback, useEffect, useRef, useState } from "react";
import { GetUnusedModels, RemoveUnusedModel, type wails } from "../api/bindings";
import { errorMessage, formatBytes } from "../format";
import { ConfirmDialog } from "./ConfirmDialog";
import "./ModelsCard.css";

// Lists what in the models folder the current speech models do not use (the models of earlier
// versions) and removes one at a time after a confirm. Nothing is removed on its own, and the card
// is hidden when there is nothing to remove.
export function ModelsCard() {
  const [items, setItems] = useState<wails.UnusedModelDTO[] | null>(null);
  const [error, setError] = useState("");
  const [asking, setAsking] = useState<wails.UnusedModelDTO | null>(null);
  const [busy, setBusy] = useState(false);
  const [removeError, setRemoveError] = useState("");
  const live = useRef(true);

  const load = useCallback(() => {
    GetUnusedModels()
      .then((list) => {
        if (!live.current) return;
        setItems(list ?? []);
        setError("");
      })
      .catch((e) => live.current && setError(errorMessage(e)));
  }, []);

  useEffect(() => {
    live.current = true;
    load();
    return () => {
      live.current = false;
    };
  }, [load]);

  const remove = async () => {
    if (!asking || busy) return;
    setBusy(true);
    setRemoveError("");
    try {
      await RemoveUnusedModel(asking.name);
      if (!live.current) return;
      setAsking(null);
      load();
    } catch (e) {
      if (live.current) setRemoveError(errorMessage(e));
    } finally {
      if (live.current) setBusy(false);
    }
  };

  if (error) {
    return (
      <section className="nt-card nt-models">
        <div className="nt-card__header">Unused speech models</div>
        <div className="nt-card nt-card--error nt-models__error" role="alert">
          {error}
        </div>
      </section>
    );
  }
  if (!items || items.length === 0) return null;

  const total = items.reduce((n, m) => n + m.bytes, 0);
  return (
    <section className="nt-card nt-models">
      <div className="nt-card__header">Unused speech models</div>
      <p className="nt-models__desc">{`Earlier versions of Phonon used these models. Nothing uses them now; together they take ${formatBytes(total)}.`}</p>
      <ul className="nt-models__list">
        {items.map((m) => (
          <li key={m.name} className="nt-models__row">
            <span className="nt-models__name">{m.name}</span>
            <span className="nt-models__size">{formatBytes(m.bytes)}</span>
            <button
              className="nt-btn nt-btn--danger-quiet"
              aria-label={`Remove ${m.name}`}
              onClick={() => {
                setRemoveError("");
                setAsking(m);
              }}
            >
              Remove
            </button>
          </li>
        ))}
      </ul>
      {asking && (
        <ConfirmDialog
          title={`Remove ${asking.name}?`}
          body={`This deletes ${asking.name} (${formatBytes(asking.bytes)}) from the models folder. Phonon does not use it.`}
          hint="This cannot be undone."
          cancelLabel="Cancel"
          confirmLabel="Remove"
          destructive
          busy={busy}
          error={removeError}
          onCancel={() => setAsking(null)}
          onConfirm={remove}
        />
      )}
    </section>
  );
}
