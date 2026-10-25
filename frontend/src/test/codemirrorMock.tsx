// Test stand-in for @uiw/react-codemirror: a textarea that calls onChange. The extensions it is
// given are loaded into a real (detached) EditorView, so a test can fire the real Mod-s keymap.
import { EditorState } from "@codemirror/state";
import { EditorView, runScopeHandlers } from "@codemirror/view";
import { useEffect, useRef } from "react";

type Props = { value: string; onChange?: (v: string) => void; extensions?: any[]; className?: string };

export default function CodeMirror({ value, onChange, extensions }: Props) {
  const view = useRef<EditorView | null>(null);
  useEffect(() => {
    const v = new EditorView({ state: EditorState.create({ doc: value, extensions: extensions ?? [] }), parent: document.createElement("div") });
    view.current = v;
    return () => v.destroy();
  }, [extensions]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <textarea
      aria-label="File contents"
      value={value}
      onChange={(e) => onChange?.(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "s" && (e.metaKey || e.ctrlKey) && view.current) {
          e.preventDefault();
          runScopeHandlers(view.current, new KeyboardEvent("keydown", { key: "s", ctrlKey: true }), "editor");
        }
      }}
    />
  );
}
