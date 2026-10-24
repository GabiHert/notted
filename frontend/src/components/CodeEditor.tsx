// Copied from github.com/brienze1/quant (frontend/src/components/CodeEditor.tsx), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
import { useMemo, useRef } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { markdown } from "@codemirror/lang-markdown";
import { EditorView, keymap } from "@codemirror/view";
import { Prec } from "@codemirror/state";
import type { Extension } from "@codemirror/state";

interface Props {
  value: string;
  onChange: (value: string) => void;
  onSave: () => void;
}

// The Ledger look, from the same CSS variables as the rest of the app (so dark mode follows).
const ledgerTheme = EditorView.theme({
  "&": { backgroundColor: "var(--card)", color: "var(--fg)", height: "100%" },
  ".cm-content": { caretColor: "var(--accent)", fontFamily: "var(--font-notes)", fontSize: "var(--notes-size)", lineHeight: "1.6" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--accent)" },
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection": { backgroundColor: "var(--sel)" },
  ".cm-gutters": { backgroundColor: "var(--side)", color: "var(--fg3-text)", border: "none" },
  ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "transparent" },
  "&.cm-focused": { outline: "none" },
});

export function CodeEditor({ value, onChange, onSave }: Props) {
  // Keep the save handler in a ref so the keymap extension stays stable.
  const onSaveRef = useRef(onSave);
  onSaveRef.current = onSave;

  const extensions = useMemo(() => {
    const exts: Extension[] = [
      // Prec.highest so Mod-S beats any default binding and never reaches the
      // browser (which would try to save the page).
      Prec.highest(
        keymap.of([
          {
            key: "Mod-s",
            preventDefault: true,
            run: () => {
              onSaveRef.current();
              return true;
            },
          },
        ]),
      ),
      markdown(),
      EditorView.lineWrapping,
      ledgerTheme,
    ];
    return exts;
  }, []);

  return <CodeMirror className="nt-editor" value={value} onChange={onChange} theme="none" extensions={extensions} height="100%" style={{ height: "100%" }} />;
}
