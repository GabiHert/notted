export const SCREENS = ["new", "new-downloading", "recording", "transcribing", "ready", "alerts", "settings", "no-project", "summarizing", "summary-error", "summarized", "save-dialog", "save-failed", "project-locked", "session-closed", "session-no-folder", "session-error", "session-open", "session-ended", "settings-empty", "settings-no-archive", "settings-save-failed", "instructions-editing", "instructions-dirty", "instructions-readonly", "instructions-discard", "instructions-save-failed", "prompt-preview", "prompt-preview-failed"] as const;
export type Screen = (typeof SCREENS)[number];

export function readScreen(search: string = window.location.search): Screen {
  const s = new URLSearchParams(search).get("screen");
  return (SCREENS as readonly string[]).includes(s ?? "") ? (s as Screen) : "recording";
}
