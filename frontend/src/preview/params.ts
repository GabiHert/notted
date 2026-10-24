export const SCREENS = ["new", "new-downloading", "recording", "transcribing", "ready", "alerts", "settings", "no-client", "summarizing", "summary-error", "summarized", "save-dialog", "save-failed", "client-locked"] as const;
export type Screen = (typeof SCREENS)[number];

export function readScreen(search: string = window.location.search): Screen {
  const s = new URLSearchParams(search).get("screen");
  return (SCREENS as readonly string[]).includes(s ?? "") ? (s as Screen) : "recording";
}
