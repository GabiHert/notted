import { vi } from "vitest";

type Handler = (...data: any[]) => void;
const handlers = new Map<string, Set<Handler>>();

export const EventsOn = vi.fn((name: string, cb: Handler) => {
  if (!handlers.has(name)) handlers.set(name, new Set());
  handlers.get(name)!.add(cb);
  return () => handlers.get(name)?.delete(cb);
});
export const EventsOff = vi.fn();

/** Delivers an event to every live listener, as the Wails runtime would. */
export function emit(name: string, payload: unknown) {
  for (const cb of [...(handlers.get(name) ?? [])]) cb(payload);
}
export function listenerCount(name: string): number {
  return handlers.get(name)?.size ?? 0;
}

export const ListMeetings = vi.fn();
export const GetMeeting = vi.fn();
export const GetTranscript = vi.fn();
export const StartRecording = vi.fn();
export const StopRecording = vi.fn();
export const GetModelStatus = vi.fn();
export const DownloadModels = vi.fn();

export function resetBindings() {
  handlers.clear();
  for (const f of [ListMeetings, GetMeeting, GetTranscript, StartRecording, StopRecording, GetModelStatus, DownloadModels, EventsOn, EventsOff]) {
    f.mockReset();
  }
  EventsOn.mockImplementation((name: string, cb: Handler) => {
    if (!handlers.has(name)) handlers.set(name, new Set());
    handlers.get(name)!.add(cb);
    return () => handlers.get(name)?.delete(cb);
  });
  ListMeetings.mockResolvedValue([]);
  GetTranscript.mockResolvedValue([]);
  DownloadModels.mockResolvedValue(undefined);
  StopRecording.mockResolvedValue(undefined);
  GetModelStatus.mockResolvedValue({ language: "pt-BR", ready: true, missing: "", downloadBytes: 0, downloading: false });
}

export function meeting(over: Record<string, unknown> = {}) {
  return {
    id: "m1",
    title: "Weekly sync",
    startedAt: "2026-10-23T14:05:00",
    endedAt: "",
    language: "en",
    clientKey: "acme",
    state: "ready",
    hasSession: false,
    folder: "",
    summaryMd: "",
    suggestionsMd: "",
    ...over,
  };
}
