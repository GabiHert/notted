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
