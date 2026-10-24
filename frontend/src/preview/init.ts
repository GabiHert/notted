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
    default:
      return { kind: "meeting", id: "fx-recording" };
  }
}
