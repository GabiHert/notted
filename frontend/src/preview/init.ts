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
    case "first-run":
    case "setup-prompt":
    case "setup-prompt-shown":
    case "setup-clone-failed":
    case "setup-profile":
    case "setup-new-profile":
    case "setup-new-project":
    case "setup-project-created":
    case "setup-delete-profile":
    case "setup-delete-confirm":
    case "setup-my-file":
    case "setup-my-file-created":
    case "setup-folder-new":
    case "setup-folder-in-repo":
    case "update-idle":
    case "update-no-source":
    case "update-running":
    case "update-failed":
    case "update-installing":
    case "permissions-allowed":
    case "permissions-mixed":
    case "permissions-denied":
    case "permissions-error":
    case "mictest-running":
    case "mictest-silent":
    case "mictest-quiet":
    case "mictest-ok":
    case "mictest-recording":
      autoDriveSettings(readScreen());
      return { kind: "settings" };
    case "transcribing":
      return { kind: "meeting", id: "fx-transcribing" };
    case "ready":
      return { kind: "meeting", id: "fx-ready" };
    case "no-project":
      return { kind: "meeting", id: "fx-no-project" };
    case "summarizing":
    case "summary-error":
      return { kind: "meeting", id: "fx-ready" };
    case "summarized":
    case "project-locked":
    case "form-text":
    case "form-html":
    case "form-choice":
    case "form-multi":
    case "form-input":
    case "form-confirm":
    case "form-permission":
    case "form-waiting":
    case "review-idle":
    case "review-running":
    case "review-done":
      scrollToSuggestions();
      return { kind: "meeting", id: "fx-summarized" };
    case "session-review-running":
      return { kind: "meeting", id: "fx-summarized" };
    case "review-session-open":
      scrollToSuggestions();
      autoOpenSession();
      return { kind: "meeting", id: "fx-summarized" };
    case "review-error":
      scrollToSuggestions();
      autoClickButton("Review suggestions");
      return { kind: "meeting", id: "fx-summarized" };
    case "waiting-dot":
      return { kind: "meeting", id: "fx-ready" };
    case "save-dialog":
    case "save-failed":
      return { kind: "meeting", id: "fx-ready" };
    case "delete-confirm":
      autoOpenDeleteConfirm();
      return { kind: "meeting", id: "fx-ready" };
    case "delete-confirm-archive":
      autoOpenDeleteConfirm();
      return { kind: "meeting", id: "fx-summarized" };
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

// Preview only: press "Delete meeting" once the page has rendered, so a screenshot URL shows the confirm.
function autoOpenDeleteConfirm() {
  let tries = 0;
  const timer = window.setInterval(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === "Delete meeting" && !b.disabled);
    if (btn) {
      window.clearInterval(timer);
      btn.click();
    } else if (++tries > 40) window.clearInterval(timer);
  }, 100);
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

// Preview only: bring the Suggestions card to the top of the window so the screenshot shows the review button.
function scrollToSuggestions() {
  let tries = 0;
  const timer = window.setInterval(() => {
    const header = Array.from(document.querySelectorAll(".nt-card__header")).find((h) => h.textContent === "Suggestions");
    if (header) {
      window.clearInterval(timer);
      header.scrollIntoView({ block: "start" });
    } else if (++tries > 40) window.clearInterval(timer);
  }, 100);
}

// Preview only: press a named button once it is enabled, so a screenshot URL shows what follows.
function autoClickButton(label: string) {
  let tries = 0;
  const timer = window.setInterval(() => {
    const btn = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === label && !b.disabled);
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
  const byLabel = (label: string) => () => click((b) => b.getAttribute("aria-label") === label);
  const edit = () => {
    const cm = document.querySelector<HTMLElement>(".cm-content");
    if (!cm) return false;
    cm.focus();
    document.execCommand("insertText", false, " Always confirm the owner of each task.");
    return true;
  };
  const choose = () => pick("#nt-preview-project", "acme")();
  const pick = (selector: string, value: string) => () => {
    const sel = document.querySelector<HTMLSelectElement>(selector);
    if (!sel || sel.options.length < 2 || sel.disabled) return false;
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(sel, value);
    sel.dispatchEvent(new Event("change", { bubbles: true }));
    return true;
  };
  const scrollTo = (selector: string) => () => {
    const el = document.querySelector(selector);
    el?.scrollIntoView({ block: "start" });
    return Boolean(el);
  };
  const typeInto = (selector: string, value: string) => () => {
    const input = document.querySelector<HTMLInputElement>(selector);
    if (!input) return false;
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    return true;
  };
  const newProject = [open("New project…"), typeInto("#nt-new-project", "initech-web")];
  const steps: Record<string, Array<() => boolean>> = {
    "instructions-editing": [open("acme.md")],
    "instructions-dirty": [open("acme.md"), edit],
    "instructions-readonly": [() => click((b) => b.textContent?.startsWith("base.md") ?? false)],
    "instructions-discard": [open("acme.md"), edit, open("ana.md")],
    "instructions-save-failed": [open("acme.md"), edit, open("Save")],
    "settings-save-failed": [open("Save settings")],
    "setup-clone-failed": [
      open("Clone the archive from GitHub instead"),
      typeInto("#nt-clone-address", "git@example.invalid:acme/meetings-archive.git"),
      typeInto("#nt-clone-parent", "/Users/ana/code"),
      open("Clone archive"),
    ],
    "setup-new-profile": [pick("#nt-profile", "__new__"), typeInto("#nt-new-profile", "carla")],
    "setup-new-project": newProject,
    "setup-project-created": [...newProject, open("Create project")],
    "setup-delete-profile": [open("Delete a profile…")],
    "setup-delete-confirm": [open("Delete a profile…"), byLabel("Delete profile bruno")],
    "setup-my-file-created": [byLabel("Create your file for globex")],
    "setup-folder-new": [byLabel("Choose the meetings archive folder")],
    "setup-folder-in-repo": [byLabel("Choose the meetings archive folder")],
    "update-idle": [scrollTo(".nt-update")],
    "update-no-source": [scrollTo(".nt-update")],
    "update-running": [scrollTo(".nt-update")],
    "update-failed": [scrollTo(".nt-update")],
    "update-installing": [scrollTo(".nt-update")],
    "permissions-allowed": [scrollTo(".nt-perms")],
    "permissions-mixed": [scrollTo(".nt-perms")],
    "permissions-denied": [scrollTo(".nt-perms")],
    "permissions-error": [scrollTo(".nt-perms")],
    "mictest-running": [scrollTo(".nt-perms"), open("Test microphone")],
    "mictest-silent": [scrollTo(".nt-perms"), open("Test microphone")],
    "mictest-quiet": [scrollTo(".nt-perms"), open("Test microphone")],
    "mictest-ok": [scrollTo(".nt-perms"), open("Test microphone")],
    "mictest-recording": [scrollTo(".nt-perms")],
    "setup-prompt": [open("Copy setup prompt")],
    "setup-prompt-shown": [open("Show prompt")],
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
