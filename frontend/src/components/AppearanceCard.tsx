import { useCallback, useEffect, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { DeleteTheme, ExportTheme, GetSelectedTheme, ImportVSCodeTheme, ListThemes, SelectTheme } from "../api/bindings";
import { errorMessage } from "../format";
import { applyTheme, THEME_TOKENS, type Theme } from "../theme";
import { ConfirmDialog } from "./ConfirmDialog";
import "./AppearanceCard.css";

const HELP = "Oscilloscope follows macOS light and dark. Import a VS Code colour theme (.json or .jsonc) to add your own; colours it lacks are derived and kept readable.";
const SYSTEM = "system";

/** Inline custom properties that paint a subtree in a theme's colours. */
function themeVars(tokens: Record<string, string> | undefined): CSSProperties {
  const s: Record<string, string> = {};
  for (const k of THEME_TOKENS) if (tokens?.[k]) s[`--${k}`] = tokens[k];
  return s as CSSProperties;
}

function SwatchHalf({ tokens }: { tokens: Record<string, string> | undefined }) {
  return (
    <span className="nt-swatch__half" style={themeVars(tokens)}>
      <span className="nt-swatch__bar" />
      <span className="nt-swatch__line" />
      <span className="nt-swatch__line nt-swatch__line--short" />
      <span className="nt-swatch__accent" />
    </span>
  );
}

/** A small preview of a theme: its surfaces, text and accent. The system theme shows dark and light side by side. */
function Swatch({ theme, list }: { theme: Theme; list: Theme[] }) {
  const halves = theme.id === SYSTEM ? ["oscilloscope-dark", "oscilloscope-light"].map((id) => list.find((t) => t.id === id)?.tokens) : [theme.tokens];
  return (
    <span className="nt-swatch" aria-hidden="true">
      {halves.map((tokens, i) => (
        <SwatchHalf key={i} tokens={tokens} />
      ))}
    </span>
  );
}

// The card loads the theme list and the chosen theme itself; results that arrive after unmount are dropped.
export function AppearanceCard() {
  const [themes, setThemes] = useState<Theme[]>([]);
  const [selected, setSelected] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [toDelete, setToDelete] = useState<Theme | null>(null);
  const [deleteError, setDeleteError] = useState("");
  const live = useRef(true);

  const reload = useCallback(async () => {
    const [list, sel] = await Promise.all([ListThemes(), GetSelectedTheme()]);
    if (!live.current) return;
    setThemes(list ?? []);
    setSelected(sel.id);
    applyTheme(sel);
  }, []);

  useEffect(() => {
    live.current = true;
    reload().catch((e) => live.current && setError(errorMessage(e)));
    return () => {
      live.current = false;
    };
  }, [reload]);

  const run = async (f: () => Promise<void>) => {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await f();
    } catch (e) {
      if (live.current) setError(errorMessage(e));
    } finally {
      if (live.current) setBusy(false);
    }
  };

  const pick = (t: Theme) =>
    run(async () => {
      const chosen = await SelectTheme(t.id);
      if (!live.current) return;
      setSelected(chosen.id);
      applyTheme(chosen);
    });

  const importTheme = () =>
    run(async () => {
      const t = await ImportVSCodeTheme();
      if (!t?.id || !live.current) return; // cancelled
      const chosen = await SelectTheme(t.id);
      await reload();
      if (live.current) {
        applyTheme(chosen);
        setNotice(`Imported ${t.name}.`);
      }
    });

  const exportTheme = (t: Theme) =>
    run(async () => {
      const path = await ExportTheme(t.id);
      if (path && live.current) setNotice(`Exported ${t.name} to ${path}.`);
    });

  const confirmDelete = async () => {
    if (!toDelete) return;
    setBusy(true);
    setDeleteError("");
    try {
      await DeleteTheme(toDelete.id);
      if (!live.current) return;
      setToDelete(null);
      await reload();
    } catch (e) {
      if (live.current) setDeleteError(errorMessage(e));
    } finally {
      if (live.current) setBusy(false);
    }
  };

  return (
    <section className="nt-card nt-appearance">
      <div className="nt-card__header">Appearance</div>
      <p className="nt-appearance__desc" id="nt-appearance-help">
        {HELP}
      </p>
      <div className="nt-appearance__list" role="radiogroup" aria-label="Theme" aria-describedby="nt-appearance-help">
        {themes.map((t) => (
          <div className={"nt-appearance__row" + (t.id === selected ? " nt-appearance__row--selected" : "")} key={t.id}>
            <label className="nt-appearance__option">
              <input type="radio" name="nt-theme" value={t.id} checked={t.id === selected} disabled={busy} onChange={() => void pick(t)} />
              <Swatch theme={t} list={themes} />
              <span className="nt-appearance__name">{t.name}</span>
              {!t.builtin && <span className="nt-appearance__mode">Custom, {t.mode}</span>}
            </label>
            {!t.builtin && (
              <span className="nt-appearance__actions">
                <button className="nt-btn" disabled={busy} onClick={() => void exportTheme(t)} aria-label={`Export ${t.name}`}>
                  Export
                </button>
                <button
                  className="nt-btn nt-btn--danger-quiet"
                  disabled={busy}
                  onClick={() => {
                    setDeleteError("");
                    setToDelete(t);
                  }}
                  aria-label={`Delete ${t.name}`}
                >
                  Delete
                </button>
              </span>
            )}
          </div>
        ))}
      </div>
      <div className="nt-appearance__footer">
        <button className="nt-btn" disabled={busy} onClick={() => void importTheme()}>
          Import VS Code theme…
        </button>
        {notice && (
          <span className="nt-appearance__notice" role="status">
            {notice}
          </span>
        )}
      </div>
      {error && (
        <div className="nt-card nt-card--error nt-appearance__error" role="alert">
          {error}
        </div>
      )}
      {toDelete && (
        <ConfirmDialog
          title="Delete theme?"
          body={`Delete the theme ${toDelete.name}? Its file in Phonon's themes folder is removed.${toDelete.id === selected ? " Phonon goes back to Oscilloscope (match system)." : ""}`}
          cancelLabel="Cancel"
          confirmLabel="Delete theme"
          destructive
          busy={busy}
          error={deleteError}
          onCancel={() => setToDelete(null)}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </section>
  );
}
