// Copied from github.com/brienze1/quant (frontend/src/components/TerminalPane.tsx), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
import { useEffect, useId, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { EventsOff, EventsOn, ResizeSession, WriteSession } from "../api/bindings";
import "./TerminalPane.css";

type Props = {
  meetingId: string;
  /** Bumped once the backend session is running, so the pane reports its real size. */
  sizeSyncKey: number;
  onExit: (code: number) => void;
  /** False until this pane's own StartSession is sent; events before that belong to an earlier instance. */
  accepts: () => boolean;
};

function readTheme(): Record<string, string> {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string) => css.getPropertyValue(name).trim();
  return { background: v("--card"), foreground: v("--fg"), cursor: v("--accent"), selectionBackground: v("--sel") };
}

const FOCUSABLE = 'a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';

// Moves focus to the next or previous control outside the terminal, wrapping at the ends.
function moveFocusOut(container: HTMLElement, backwards: boolean) {
  const items = Array.from(document.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => !container.contains(el));
  if (items.length === 0) return;
  const after = items.filter((el) => container.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING);
  const before = items.filter((el) => container.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_PRECEDING);
  const target = backwards ? (before[before.length - 1] ?? items[items.length - 1]) : (after[0] ?? items[0]);
  target.focus();
}

export function TerminalPane({ meetingId, sizeSyncKey, onExit, accepts }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const hintId = useId();
  const onExitRef = useRef(onExit);
  onExitRef.current = onExit;
  const acceptsRef = useRef(accepts);
  acceptsRef.current = accepts;

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const fit = new FitAddon();
    const term = new Terminal({
      fontFamily: "ui-monospace, Menlo, monospace",
      fontSize: 12.5,
      cursorBlink: true,
      scrollback: 5000,
      theme: readTheme(),
    });
    term.loadAddon(fit);
    term.open(container);
    fit.fit();
    term.focus();
    // Tab and Shift+Tab belong to the session, so Ctrl+Tab and Ctrl+Shift+Tab are the way out.
    term.attachCustomKeyEventHandler((e: KeyboardEvent) => {
      if (e.ctrlKey && !e.altKey && !e.metaKey && e.key === "Tab") {
        if (e.type === "keydown") moveFocusOut(container, e.shiftKey);
        return false;
      }
      return true;
    });
    termRef.current = term;
    fitRef.current = fit;

    // The person's keystrokes are the only thing ever written to the session.
    term.onData((d) => void WriteSession(meetingId, d).catch(() => {}));
    term.onResize(({ cols, rows }) => void ResizeSession(meetingId, cols, rows).catch(() => {}));

    const dataEvent = "terminal:data:" + meetingId;
    const exitEvent = "terminal:exit:" + meetingId;
    EventsOn(dataEvent, (d: string) => acceptsRef.current() && term.write(d));
    EventsOn(exitEvent, (e: { code: number }) => acceptsRef.current() && onExitRef.current(e?.code ?? 0));

    const observer = new ResizeObserver(() => {
      try {
        fit.fit();
      } catch {
        // Ignore fit errors during disposal.
      }
    });
    observer.observe(container);

    const media = typeof window.matchMedia === "function" ? window.matchMedia("(prefers-color-scheme: dark)") : null;
    const onScheme = () => {
      term.options.theme = readTheme();
    };
    media?.addEventListener?.("change", onScheme);

    return () => {
      media?.removeEventListener?.("change", onScheme);
      observer.disconnect();
      EventsOff(dataEvent);
      EventsOff(exitEvent);
      term.dispose();
      termRef.current = null;
      fitRef.current = null;
    };
  }, [meetingId]);

  useEffect(() => {
    const term = termRef.current;
    if (sizeSyncKey === 0 || !term) return;
    try {
      fitRef.current?.fit();
    } catch {
      // Ignore fit errors during disposal.
    }
    void ResizeSession(meetingId, term.cols, term.rows).catch(() => {});
  }, [sizeSyncKey, meetingId]);

  return (
    <>
      <div className="nt-terminal" ref={containerRef} data-testid="terminal" aria-describedby={hintId} />
      <p className="nt-terminal__hint" id={hintId}>
        Press Ctrl+Tab to leave the terminal.
      </p>
    </>
  );
}
