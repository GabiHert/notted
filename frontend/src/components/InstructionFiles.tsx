import { useEffect, useState } from "react";
import { ListInstructionFiles } from "../api/bindings";
import { errorMessage } from "../format";
import { ConfirmDialog } from "./ConfirmDialog";
import { FileEditor } from "./FileEditor";
import "./InstructionFiles.css";

type File = { path: string; kind: string; readOnly: boolean };

const GROUPS: [string, string][] = [
  ["base", "Base prompt"],
  ["readme", "Guide"],
  ["client", "Clients"],
  ["person", "People"],
  ["person-client", "Per client"],
  ["local", "Local overrides"],
];

function itemName(f: File): string {
  const parts = f.path.split("/").filter(Boolean);
  return f.kind === "person-client" ? parts.slice(-2).join("/") : (parts.pop() ?? f.path);
}

export function InstructionFiles({ onDirtyPath }: { onDirtyPath?: (path: string | null) => void } = {}) {
  const [files, setFiles] = useState<File[] | null>(null);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<File | null>(null);
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState<File | null>(null);

  useEffect(() => {
    let live = true;
    ListInstructionFiles()
      .then((list) => live && setFiles(list ?? []))
      .catch((e) => live && setListError(errorMessage(e)));
    return () => {
      live = false;
    };
  }, []);

  const dirtyPath = dirty && selected ? selected.path : null;
  useEffect(() => {
    onDirtyPath?.(dirtyPath);
  }, [dirtyPath]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => onDirtyPath?.(null), []); // eslint-disable-line react-hooks/exhaustive-deps

  const choose = (f: File) => {
    if (selected && f.path === selected.path) return;
    if (dirty) setPending(f);
    else setSelected(f);
  };
  const discard = () => {
    setDirty(false);
    setSelected(pending);
    setPending(null);
  };

  return (
    <section className="nt-card">
      <div className="nt-card__header">Instruction files</div>
      {listError && (
        <div className="nt-card nt-card--error" role="alert">
          {listError}
        </div>
      )}
      <div className="nt-files">
        <nav className="nt-files__list" aria-label="Instruction files">
          {GROUPS.map(([kind, label]) => {
            const items = (files ?? []).filter((f) => f.kind === kind);
            if (items.length === 0) return null;
            return (
              <div className="nt-files__group" key={kind}>
                <div className="nt-files__group-label nt-card__header">{label}</div>
                {items.map((f) => (
                  <button
                    key={f.path}
                    className={"nt-files__item" + (selected?.path === f.path ? " nt-files__item--selected" : "")}
                    aria-current={selected?.path === f.path ? "true" : undefined}
                    onClick={() => choose(f)}
                  >
                    <span className="nt-files__name">{itemName(f)}</span>
                    {f.readOnly && <span className="nt-files__badge">Read-only</span>}
                  </button>
                ))}
              </div>
            );
          })}
        </nav>
        <div className="nt-files__editor">
          {selected ? <FileEditor key={selected.path} path={selected.path} readOnly={selected.readOnly} onDirtyChange={setDirty} /> : <p className="nt-files__empty">Choose a file to read or edit it.</p>}
        </div>
      </div>
      {pending && selected && (
        <ConfirmDialog
          title="Discard unsaved changes?"
          body={`Your changes to ${selected.path} are not saved.`}
          cancelLabel="Keep editing"
          confirmLabel="Discard"
          onCancel={() => setPending(null)}
          onConfirm={discard}
        />
      )}
    </section>
  );
}
