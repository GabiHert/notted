const dateFmt = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export function formatDate(iso: string): string {
  return dateFmt.format(new Date(iso));
}

export function languageName(code: string): string {
  if (code === "pt-BR") return "Portuguese (Brazil)";
  if (code === "en") return "English";
  return code;
}

const states: Record<string, string> = {
  recording: "Recording",
  transcribing: "Transcribing",
  ready: "Ready",
  summarized: "Summarized",
};

export function stateName(state: string): string {
  return states[state] ?? state;
}

export function speakerName(track: string): string {
  return track === "mic" ? "You" : "Others";
}

const pad = (n: number) => String(n).padStart(2, "0");

export function clock(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  return `${pad(Math.floor(s / 3600))}:${pad(Math.floor((s % 3600) / 60))}:${pad(s % 60)}`;
}

export function elapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  return `${pad(Math.floor(s / 60))}:${pad(s % 60)}`;
}

export function errorMessage(e: unknown): string {
  if (typeof e === "string") return e;
  if (e instanceof Error) return e.message;
  return String(e);
}
