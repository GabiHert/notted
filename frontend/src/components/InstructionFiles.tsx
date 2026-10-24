import { useCallback, useEffect, useRef, useState } from "react";
import { ListInstructionFiles } from "../api/bindings";
import { errorMessage } from "../format";
import { ConfirmDialog } from "./ConfirmDialog";
import { FileEditor } from "./FileEditor";
import "./InstructionFiles.css";

type File = { path: string; kind: string; readOnly: boolean };

const GROUPS: [string, string][] = [
  ["base", "Base prompt"],
  ["readme", "Guide"],
  ["project", "Projects"],
  ["person", "People"],
  ["person-project", "Per project"],
  ["local", "Local overrides"],
];

function itemName(f: File): string {
  const parts = f.path.split("/").filter(Boolean);
  return f.kind === "person-project" ? parts.slice(-2).join("/") : (parts.pop() ?? f.path);
}

type OpenRequest = { path: string; seq: number };
type Props = { onDirtyPath?: (path: string | null) => void; archivePath?: string; openRequest?: OpenRequest | null };

export function InstructionFiles({ onDirtyPath, archivePath = "", openRequest = null }: Props = {}) {
  const [files, setFiles] = useState<File[] | null>(null);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<File | null>(null);
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState<File | null>(null);
  const card = useRef<HTMLElement>(null);
  const request = useRef(0);
  const live = useRef(true);

  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  // Only the newest list request may apply its result; `then` runs after the list is stored.
  const load = useCallback((then?: (list: File[]) => void) => {
    const id = ++request.current;
    ListInstructionFiles()
      .then((list) => {
        if (!live.current || request.current !== id) return;
        setFiles(list ?? []);
        setListError("");
        then?.(list ?? []);
      })
      .catch((e) => {
        if (live.current && request.current === id) setListError(errorMessage(e));
      });
  }, []);

  useEffect(() => {
    load();
  }, [archivePath, load]);

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
  const chooseNow = useRef(choose);
  chooseNow.current = choose;
  const seq = openRequest?.seq;
  const path = openRequest?.path;
  useEffect(() => {
    if (seq === undefined || path === undefined) return;
    load((list) => {
      chooseNow.current(list.find((f) => f.path === path) ?? { path, kind: "", readOnly: false });
      card.current?.scrollIntoView?.({ block: "start" });
    });
  }, [seq]); // eslint-disable-line react-hooks/exhaustive-deps
  const discard = () => {
    setDirty(false);
    setSelected(pending);
    setPending(null);
  };

  return (
    <section className="nt-card" ref={card}>
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
