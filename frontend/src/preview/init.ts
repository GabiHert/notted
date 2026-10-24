// Preview mode only (vite --mode preview). Never part of the production bundle.
import type { View } from "../components/Sidebar";
import { readScreen } from "./params";

export function initPreview(): View {
  const theme = new URLSearchParams(window.location.search).get("theme");
  if (theme === "light" || theme === "dark") document.documentElement.setAttribute("data-theme", theme);
  switch (readScreen()) {
    case "new":
    case "new-downloading":
      return { kind: "new" };
    case "settings":
    case "settings-empty":
    case "settings-no-archive":
    case "settings-save-failed":
    case "instructions-editing":
    case "instructions-dirty":
    case "instructions-readonly":
    case "instructions-discard":
    case "instructions-save-failed":
    case "prompt-preview":
    case "prompt-preview-failed":
      autoDriveSettings(readScreen());
      return { kind: "settings" };
    case "transcribing":
      return { kind: "meeting", id: "fx-transcribing" };
    case "ready":
      return { kind: "meeting", id: "fx-ready" };
    case "no-client":
      return { kind: "meeting", id: "fx-no-client" };
    case "summarizing":
    case "summary-error":
      return { kind: "meeting", id: "fx-ready" };
    case "summarized":
    case "client-locked":
      return { kind: "meeting", id: "fx-summarized" };
    case "save-dialog":
    case "save-failed":
      return { kind: "meeting", id: "fx-ready" };
    case "session-closed":
    case "session-no-folder":
    case "session-error":
      return { kind: "meeting", id: "fx-session" };
    case "session-open":
    case "session-ended":
      autoOpenSession();
      return { kind: "meeting", id: "fx-session" };
    default:
      return { kind: "meeting", id: "fx-recording" };
  }
}

// Preview only: press "Open session" once the card has rendered, so a screenshot URL shows the terminal.
function autoOpenSession() {
  let tries = 0;
  const timer = window.setInterval(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === "Open session" && !b.disabled);
    if (btn) {
      window.clearInterval(timer);
      btn.click();
    } else if (++tries > 40) window.clearInterval(timer);
  }, 100);
}

// Preview only: drive the Settings view into the state a screenshot URL names.
function autoDriveSettings(screen: string) {
  const click = (match: (b: HTMLButtonElement) => boolean) => {
    const btn = Array.from(document.querySelectorAll("button")).find((b) => match(b) && !b.disabled);
    btn?.click();
    return Boolean(btn);
  };
  const open = (name: string) => () => click((b) => b.textContent === name);
  const edit = () => {
    const cm = document.querySelector<HTMLElement>(".cm-content");
    if (!cm) return false;
    cm.focus();
    document.execCommand("insertText", false, " Always confirm the owner of each task.");
    return true;
  };
  const choose = () => {
    const sel = document.querySelector<HTMLSelectElement>("#nt-preview-client");
    if (!sel || sel.options.length < 2) return false;
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(sel, "acme");
    sel.dispatchEvent(new Event("change", { bubbles: true }));
    return true;
  };
  const steps: Record<string, Array<() => boolean>> = {
    "instructions-editing": [open("acme.md")],
    "instructions-dirty": [open("acme.md"), edit],
    "instructions-readonly": [() => click((b) => b.textContent?.startsWith("base.md") ?? false)],
    "instructions-discard": [open("acme.md"), edit, open("ana.md")],
    "instructions-save-failed": [open("acme.md"), edit, open("Save")],
    "settings-save-failed": [open("Save settings")],
    "prompt-preview": [choose, open("Preview prompt")],
    "prompt-preview-failed": [choose, open("Preview prompt")],
  };
  const list = steps[screen];
  if (!list) return;
  let i = 0;
  let tries = 0;
  const timer = window.setInterval(() => {
    if (list[i]()) i++;
    if (i >= list.length || ++tries > 60) window.clearInterval(timer);
  }, 200);
}
