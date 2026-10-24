// Preview-mode stand-in for wailsjs/go/wails/App: fixture data only, generic names.
import { emit } from "./mockRuntime";
import { readScreen } from "./params";

const screen = readScreen();
const minutes = (n: number) => new Date(Date.now() - n * 60_000).toISOString();

const meetings = [
  { id: "fx-recording", title: "Weekly sync", startedAt: minutes(5), endedAt: "", language: "en", projectKey: "acme", state: "recording", hasSession: false, folder: "", summaryMd: "", suggestionsMd: "" },
  { id: "fx-transcribing", title: "Planning review", startedAt: "2026-10-22T16:00:00", endedAt: "2026-10-22T16:42:00", language: "pt-BR", projectKey: "acme", state: "transcribing", hasSession: false, folder: "", summaryMd: "", suggestionsMd: "" },
  { id: "fx-ready", title: "Kickoff with the acme team", startedAt: "2026-10-21T10:30:00", endedAt: "2026-10-21T11:15:00", language: "en", projectKey: "acme", state: "ready", hasSession: false, folder: "", summaryMd: "", suggestionsMd: "" },
  { id: "fx-no-project", title: "Intro call", startedAt: "2026-10-20T09:00:00", endedAt: "2026-10-20T09:30:00", language: "en", projectKey: "", state: "ready", hasSession: false, folder: "", summaryMd: "", suggestionsMd: "" },
  {
    id: "fx-summarized",
    title: "Budget review",
    startedAt: "2026-10-19T15:00:00",
    endedAt: "2026-10-19T15:50:00",
    language: "en",
    projectKey: "acme",
    state: "summarized",
    hasSession: screen === "project-locked",
    folder: "meetings/acme/2026/2026-10-19-budget-review",
    summaryMd:
      "# Summary: Budget review\n\nThe team went through the quarter's spending and agreed on the next steps.\n\n## Decisions\n\n- Keep the current vendor until March\n- Move the review to **Fridays**\n\n## Open questions\n\n1. Who owns the `forecast.xlsx` update?\n2. Is the second invoice already paid?\n",
    suggestionsMd: "# Suggestions\n\n- [ ] Send the draft budget to the acme team\n- [ ] Book the Friday review\n- [x] Share last week's notes\n",
  },
  {
    id: "fx-session",
    title: "Budget review",
    startedAt: "2026-10-19T15:00:00",
    endedAt: "2026-10-19T15:50:00",
    language: "en",
    projectKey: "acme",
    state: "summarized",
    hasSession: true,
    folder: "meetings/acme/2026/2026-10-19-budget-review",
    summaryMd: "# Summary: Budget review\n\nThe team went through the quarter's spending and agreed on the next steps.\n",
    suggestionsMd: "# Suggestions\n\n- [ ] Send the draft budget to the acme team\n",
  },
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

const projectsEmpty = false;
const projectFixtures = ["acme", "globex", "initech"];
function afterOpen(fn: () => void) {
  setTimeout(fn, 500);
}
if (screen === "summarizing") {
  afterOpen(() => emit("summary:state", { meetingId: "fx-ready", pass: "suggestions", state: "running" }));
}
if (screen === "summary-error") {
  afterOpen(() =>
    emit("summary:state", { meetingId: "fx-ready", pass: "summary", state: "error", message: "Claude Code stopped with an error:\nnot logged in. Run claude in a terminal and sign in." }),
  );
}
if (screen === "save-dialog" || screen === "save-failed") {
  afterOpen(() =>
    emit("export:ready", {
      meetingId: "fx-ready",
      folder: "meetings/acme/2026/2026-10-21-kickoff-with-the-acme-team",
      files: ["meetings/acme/2026/2026-10-21-kickoff-with-the-acme-team/transcript.md", "meetings/acme/2026/2026-10-21-kickoff-with-the-acme-team/summary.md", "meetings/acme/2026/2026-10-21-kickoff-with-the-acme-team/suggestions.md"],
    }),
  );
}

const PROMPT =
  "\x1b[1m> /suggest\x1b[0m\r\n\r\nI can write the suggestions to the meeting folder.\r\n\r\n" +
  "Do you want to create suggestions.md?\r\n \x1b[36m1.\x1b[0m Yes\r\n  2. Yes, and allow all edits this session\r\n  3. No\r\n\r\n";
export async function GetSettings() {
  if (screen === "settings-empty") return { profile: "", checkoutPath: "", projectRoots: {} };
  if (screen.startsWith("settings") || screen.startsWith("instructions") || screen.startsWith("prompt"))
    return { profile: "ana", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } };
  return { profile: "", checkoutPath: screen === "session-no-folder" ? "" : "/archive", projectRoots: {} };
}
export async function SetSettings(_s: unknown) {
  if (screen === "settings-save-failed") throw new Error("The archive folder does not exist: /archive");
}

const instructionFiles = [
  { path: "prompts/base.md", kind: "base", readOnly: true },
  { path: "instructions/README.md", kind: "readme", readOnly: false },
  { path: "instructions/projects/acme.md", kind: "project", readOnly: false },
  { path: "instructions/projects/globex.md", kind: "project", readOnly: false },
  { path: "instructions/people/ana.md", kind: "person", readOnly: false },
  { path: "instructions/people/ana/acme.md", kind: "person-project", readOnly: false },
  { path: "instructions/local/notes.md", kind: "local", readOnly: false },
];
const instructionText: Record<string, string> = {
  "prompts/base.md": "# Base prompt\n\nSummarize the meeting in short sections.\n\n- Decisions first\n- Then open questions\n",
  "instructions/README.md": "# Guide\n\nOne file per project, one per person.\n",
  "instructions/projects/acme.md": "# Acme\n\nUse the acme glossary. Keep the summary under one page.\n",
  "instructions/projects/globex.md": "# Globex\n\nName the sponsor of each decision.\n",
  "instructions/people/ana.md": "# Ana\n\nWrite in short sentences.\n",
  "instructions/people/ana/acme.md": "# Ana for Acme\n\nList the follow-ups last.\n",
  "instructions/local/notes.md": "# Local notes\n",
};
export async function ListInstructionFiles() {
  return instructionFiles;
}
export async function ReadInstructionFile(path: string) {
  return instructionText[path] ?? "";
}
export async function WriteInstructionFile(_path: string, _text: string) {
  if (screen === "instructions-save-failed") throw new Error("Could not write instructions/projects/acme.md: permission denied");
}
export async function PreviewPrompt(key: string) {
  if (screen === "prompt-preview-failed") throw new Error(`There is no instructions/projects/${key}.md in the archive folder.`);
  return `${instructionText["prompts/base.md"]}\n---\n${instructionText[`instructions/projects/${key}.md`] ?? ""}\n---\n${instructionText["instructions/people/ana.md"]}`;
}
export async function StartSession(id: string) {
  if (screen === "session-error") throw new Error("Claude Code is not installed. Install it and try again.");
  setTimeout(() => emit("terminal:data:" + id, PROMPT), 300);
  if (screen === "session-ended") setTimeout(() => emit("terminal:exit:" + id, { code: 0 }), 1200);
}
export async function WriteSession(_id: string, _data: string) {}
export async function ResizeSession(_id: string, _cols: number, _rows: number) {}
export async function StopSession(_id: string) {}

export async function ListProjects() {
  if (screen === "settings-no-archive" || screen === "settings-empty") throw new Error("Set the archive folder to list its projects.");
  return projectsEmpty ? [] : projectFixtures;
}
export async function SetMeetingProject(_id: string, _key: string) {}
export async function GenerateSummary(_id: string) {}
export async function CommitAndPushMeeting(_id: string) {
  if (screen === "save-failed") throw new Error("The commit was rejected:\npre-commit hook failed: trailing whitespace in summary.md");
  return { commit: "abc1234" };
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
