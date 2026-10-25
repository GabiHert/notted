export const SCREENS = ["new", "new-downloading", "recording", "transcribing", "ready", "alerts", "settings", "no-project", "summarizing", "summary-error", "summarized", "save-dialog", "save-failed", "project-locked", "session-closed", "session-no-folder", "session-error", "session-open", "session-ended", "settings-empty", "settings-no-archive", "settings-save-failed", "instructions-editing", "instructions-dirty", "instructions-readonly", "instructions-discard", "instructions-save-failed", "prompt-preview", "prompt-preview-failed", "first-run", "setup-clone-failed", "setup-profile", "setup-new-profile", "setup-new-project", "setup-project-created", "setup-delete-profile", "setup-delete-confirm", "setup-my-file", "setup-my-file-created", "setup-folder-new", "setup-folder-in-repo", "update-idle", "update-no-source", "update-running", "update-failed", "update-installing", "permissions-allowed", "permissions-mixed", "permissions-denied", "permissions-error", "mictest-running", "mictest-silent", "mictest-quiet", "mictest-ok", "mictest-recording", "mic-warning", "delete-confirm", "delete-confirm-archive"] as const;
export type Screen = (typeof SCREENS)[number];

export function readScreen(search: string = window.location.search): Screen {
  const s = new URLSearchParams(search).get("screen");
  return (SCREENS as readonly string[]).includes(s ?? "") ? (s as Screen) : "recording";
}
