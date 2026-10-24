// Copied from github.com/brienze1/quant (frontend/src/components/FileTabPanel.tsx), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
import { useEffect, useRef, useState } from "react";
import { ReadInstructionFile, WriteInstructionFile } from "../api/bindings";
import { errorMessage } from "../format";
import { CodeEditor } from "./CodeEditor";
import { MarkdownView } from "./MarkdownView";

type Props = { path: string; readOnly: boolean; onDirtyChange: (dirty: boolean) => void };

// One instruction file: read -> savedContent + draft; dirty = draft !== savedContent; save -> write -> savedContent = what was written.
// The parent keys this component by path, so every async result below is checked against `live`.
export function FileEditor({ path, readOnly, onDirtyChange }: Props) {
  const [loaded, setLoaded] = useState(false);
  const [savedContent, setSavedContent] = useState("");
  const [draft, setDraft] = useState("");
  const [mode, setMode] = useState<"edit" | "preview">(readOnly ? "preview" : "edit");
  const [saving, setSaving] = useState(false);
  const [justSaved, setJustSaved] = useState(false);
  const [error, setError] = useState("");
  const live = useRef(true);

  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  useEffect(() => {
    let current = true;
    setLoaded(false);
    ReadInstructionFile(path)
      .then((text) => {
        if (!current || !live.current) return;
        setSavedContent(text);
        setDraft(text);
        setLoaded(true);
      })
      .catch((e) => {
        if (current && live.current) setError(errorMessage(e));
      });
    return () => {
      current = false;
    };
  }, [path]);

  const dirty = loaded && !readOnly && draft !== savedContent;
  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty]); // eslint-disable-line react-hooks/exhaustive-deps

  const save = () => {
    if (readOnly || !loaded || saving || draft === savedContent) return;
    const text = draft;
    setSaving(true);
    setError("");
    setJustSaved(false);
    WriteInstructionFile(path, text)
      .then(() => {
        if (!live.current) return;
        setSavedContent(text);
        setJustSaved(true);
      })
      .catch((e) => {
        if (live.current) setError(errorMessage(e));
      })
      .finally(() => {
        if (live.current) setSaving(false);
      });
  };

  const change = (text: string) => {
    setDraft(text);
    setJustSaved(false);
  };

  return (
    <div className="nt-editor-pane">
      <div className="nt-editor-pane__head">
        <span className="nt-editor-pane__path">{path}</span>
        {!readOnly && (
          <div className="nt-toggle" role="group" aria-label="View">
            <button className={"nt-toggle__btn" + (mode === "edit" ? " nt-toggle__btn--on" : "")} aria-pressed={mode === "edit"} onClick={() => setMode("edit")}>
              Edit
            </button>
            <button className={"nt-toggle__btn" + (mode === "preview" ? " nt-toggle__btn--on" : "")} aria-pressed={mode === "preview"} onClick={() => setMode("preview")}>
              Preview
            </button>
          </div>
        )}
        <span className="nt-editor-pane__spacer" />
        {dirty && <span className="nt-editor-pane__marker">Unsaved changes</span>}
        {justSaved && !dirty && <span className="nt-editor-pane__marker">Saved.</span>}
        {!readOnly && (
          <button className="nt-btn nt-btn--primary" onClick={save} disabled={!dirty || saving}>
            Save
          </button>
        )}
      </div>
      {readOnly && <p className="nt-editor-pane__note">This file is read-only in Notted.</p>}
      {error && (
        <div className="nt-card nt-card--error nt-editor-pane__error" role="alert">
          {error}
        </div>
      )}
      <div className="nt-editor-pane__body">
        {loaded &&
          (mode === "edit" && !readOnly ? (
            <CodeEditor value={draft} onChange={change} onSave={save} />
          ) : (
            <div className="nt-editor-pane__preview">
              <MarkdownView content={draft} />
            </div>
          ))}
      </div>
    </div>
  );
}
