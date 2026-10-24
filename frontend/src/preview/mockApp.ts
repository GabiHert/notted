// Preview-mode stand-in for wailsjs/go/wails/App: fixture data only, generic names.
import { emit } from "./mockRuntime";
import { readScreen } from "./params";

const screen = readScreen();
const minutes = (n: number) => new Date(Date.now() - n * 60_000).toISOString();

const meetings = [
  { id: "fx-recording", title: "Weekly sync", startedAt: minutes(5), endedAt: "", language: "en", clientKey: "acme", state: "recording", hasSession: false, folder: "", summaryMd: "", suggestionsMd: "" },
  { id: "fx-transcribing", title: "Planning review", startedAt: "2026-10-22T16:00:00", endedAt: "2026-10-22T16:42:00", language: "pt-BR", clientKey: "acme", state: "transcribing", hasSession: false, folder: "", summaryMd: "", suggestionsMd: "" },
  { id: "fx-ready", title: "Kickoff with the acme team", startedAt: "2026-10-21T10:30:00", endedAt: "2026-10-21T11:15:00", language: "en", clientKey: "acme", state: "ready", hasSession: false, folder: "", summaryMd: "", suggestionsMd: "" },
];

const transcript = [
  { id: 1, track: "mic", startMs: 2_000, endMs: 9_000, text: "Thanks for joining. Let us start with last week's open items." },
  { id: 2, track: "others", startMs: 11_000, endMs: 24_000, text: "The export is done; I only need a second review before it goes out." },
  { id: 3, track: "mic", startMs: 27_000, endMs: 33_000, text: "Good. Can you send me the draft today?" },
  { id: 4, track: "others", startMs: 36_000, endMs: 52_000, text: "Yes, I will share it right after this call, together with the notes from Tuesday." },
  { id: 5, track: "mic", startMs: 63_000, endMs: 70_000, text: "Next topic: the schedule for the next two weeks." },
];

const modelsMissing = screen === "new" || screen === "new-downloading";

if (screen === "new-downloading") {
  setTimeout(() => emit("models:progress", { language: "pt-BR", name: "whisper-small", phase: "download", bytesDone: 205_000_000, bytesTotal: 487_000_000 }), 500);
}
if (screen === "alerts") {
  setTimeout(
    () =>
      emit("recording:state", {
        meetingId: "fx-recording",
        state: "recording",
        warning: "No sound from other people has been captured for the last minute. Check that the meeting app is playing audio.",
        error: "The recorder lost the microphone. Reconnect it and start a new recording.",
      }),
    500,
  );
}

export async function ListMeetings() {
  return meetings;
}
export async function GetMeeting(id: string) {
  return meetings.find((m) => m.id === id) ?? meetings[0];
}
export async function GetTranscript(id: string) {
  return id === "fx-transcribing" ? [] : transcript;
}
export async function GetModelStatus(language: string) {
  return { language, ready: !modelsMissing, missing: modelsMissing ? "whisper-small" : "", downloadBytes: 487_400_000, downloading: false };
}
export async function StartRecording(title: string, language: string) {
  return { ...meetings[0], title: title || "Untitled", language };
}
export async function StopRecording(_id: string) {}
export async function DownloadModels(_language: string) {}
