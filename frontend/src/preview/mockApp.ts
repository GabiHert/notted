// Preview-mode stand-in for wailsjs/go/wails/App: fixture data only, generic names.
import { emit } from "./mockRuntime";
import { fixtureFor } from "./formFixtures";
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
if (screen === "mic-warning") {
  setTimeout(
    () =>
      emit("recording:state", {
        meetingId: "fx-recording",
        state: "recording",
        warning:
          "Your microphone recorded no sound (MacBook Air Microphone). If your MacBook's lid is closed, its built-in microphone is off: open the lid, or choose another input in System Settings > Sound > Input.",
      }),
    500,
  );
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
const ARCHIVE = "/Users/ana/meetings-archive";
const setupScreens = ["first-run", "setup-clone-failed", "setup-profile", "setup-new-profile", "setup-new-project", "setup-project-created", "setup-delete-profile", "setup-delete-confirm", "setup-my-file", "setup-my-file-created", "setup-folder-new", "setup-folder-in-repo"];
let projectCreated = false;
let myFileCreated = false;
export async function GetSettings() {
  if (screen === "first-run" || screen === "setup-clone-failed") return { profile: "", checkoutPath: "", projectRoots: {} };
  if (screen === "setup-profile" || screen === "setup-new-profile") return { profile: "", checkoutPath: ARCHIVE, projectRoots: {} };
  if (screen === "setup-new-project" || screen === "setup-project-created") return { profile: "ana", checkoutPath: ARCHIVE, projectRoots: { acme: "/Users/ana/work/acme" } };
  if (screen === "settings-empty") return { profile: "", checkoutPath: "", projectRoots: {} };
  if (screen.startsWith("setup-prompt")) return { profile: "ana", checkoutPath: ARCHIVE, projectRoots: { acme: "/Users/ana/work/acme" } };
  if (screen.startsWith("setup-folder")) return { profile: "ana", checkoutPath: "/Users/ana/old-archive", projectRoots: { acme: "/Users/ana/work/acme" } };
  if (screen.startsWith("setup-delete") || screen.startsWith("setup-my-file")) return { profile: "ana", checkoutPath: ARCHIVE, projectRoots: { acme: "/Users/ana/work/acme" } };
  if (screen.startsWith("settings") || screen.startsWith("instructions") || screen.startsWith("prompt") || screen.startsWith("update") || screen.startsWith("permissions") || screen.startsWith("mictest"))
    return { profile: "ana", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } };
  return { profile: "", checkoutPath: screen === "session-no-folder" ? "" : "/archive", projectRoots: {} };
}
export async function GetSetupPrompt() {
  return [
    "You are helping me finish setting up Notted, a Mac app that records my meetings and turns each one into a summary and a list of suggested tasks. Read this whole message, then talk with me to finish the setup.",
    "",
    "## My current state",
    "",
    `- Archive folder: ${ARCHIVE}`,
    "- Profile: ana",
    "- Projects in the archive: acme, globex",
    "- My files in the archive: instructions/people/ana.md, instructions/people/ana/acme.md",
    "- Projects without a file of mine: globex",
    "",
    "## What I need you to do",
    "",
    "1. Read instructions/README.md and prompts/base.md in the archive.",
    "2. Interview me one question at a time. Wait for my answer before you ask the next.",
    "3. Write only under instructions/ in the archive.",
    "4. You must never commit or push without my yes.",
  ].join("\n");
}
export async function SetSettings(_s: unknown) {
  if (screen === "settings-save-failed") throw new Error("The archive folder does not exist: /archive");
}

const SOURCE = "/Users/ana/code/notted";
const updateLines = [
  "$ git pull --ff-only --no-rebase --no-autostash",
  "Updating 2a8d067..47d92f5",
  "Fast-forward",
  " frontend/src/components/UpdateCard.tsx | 120 ++++++++++",
  " 1 file changed, 120 insertions(+)",
  "$ make build",
  "bash helpers/audiocap/build.sh",
  "built /Users/ana/code/notted/helpers/audiocap/build/audiocap",
  "Wails CLI v2.12.0",
  "• Generating bindings: Done.",
  "• Compiling frontend: Done.",
  "• Compiling application: Done.",
  "• Packaging application: Done.",
  'sign-bundle: signing with "Notted Local Signing"',
  "check-bundle: OK (signed with Notted Local Signing)",
];
const failedLines = [...updateLines.slice(0, 11), "go: github.com/acme/widgets@v1.2.0: module lookup disabled by GOPROXY=off", "make: *** [build] Error 1"];
export async function GetUpdateStatus() {
  const base = { sourcePath: SOURCE, installedApp: "/Applications/Notted.app", signing: true, state: "idle", message: "", lines: [] as string[], seq: 0 };
  if (screen === "update-no-source") return { ...base, sourcePath: "", signing: false };
  if (screen === "update-running") return { ...base, state: "building", lines: updateLines.slice(0, 11), seq: 11 };
  if (screen === "update-failed")
    return { ...base, state: "failed", message: "make build failed. Notted did not change. The output below shows why. The build runs without the network: if this update added dependencies, run make build once in Terminal in the Notted source folder, then update again.", lines: failedLines, seq: failedLines.length };
  if (screen === "update-installing") return { ...base, state: "installing", message: "Notted quits now and reopens when the new version is in place.", lines: updateLines, seq: updateLines.length };
  return base;
}
export async function SetUpdateSource(_path: string) {}
export async function StartUpdate() {}

let micAsked = false;
export async function GetPermissions() {
  if (screen === "permissions-error") throw new Error("Notted could not read the permissions: the audio recorder did not answer. Try Check again, or reinstall Notted if this keeps happening.");
  if (screen === "permissions-denied") return { microphone: "denied", screen: "denied" };
  if (screen === "permissions-mixed") return { microphone: micAsked ? "allowed" : "not_asked", screen: "denied" };
  return { microphone: "allowed", screen: "allowed" };
}
export async function AskPermissions() {
  micAsked = true;
  return GetPermissions();
}
export async function OpenPermissionSettings(_kind: string) {}

// The mic test fixtures: the preview plays the levels of each verdict, then ends the test (mictest-running never ends).
const MIC_DEVICE = "MacBook Air Microphone";
const LID = "If your MacBook's lid is closed, its built-in microphone is off: open the lid, or choose another input in System Settings > Sound > Input.";
const micVerdicts: Record<string, { verdict: string; message: string; rms: number; peak: number }> = {
  "mictest-silent": { verdict: "silent", message: `No sound from ${MIC_DEVICE}. ${LID}`, rms: -120, peak: -120 },
  "mictest-quiet": { verdict: "quiet", message: `Very quiet. Speak closer to ${MIC_DEVICE}, or choose another input in System Settings > Sound > Input.`, rms: -58, peak: -46 },
  "mictest-ok": { verdict: "ok", message: "The microphone works.", rms: -32, peak: -14 },
  "mictest-running": { verdict: "ok", message: "The microphone works.", rms: -34, peak: -16 },
};
let micTimer: ReturnType<typeof setInterval> | undefined;
export async function StartMicTest() {
  if (screen === "mictest-recording") throw new Error("Stop the recording to test the microphone.");
  const fx = micVerdicts[screen] ?? micVerdicts["mictest-ok"];
  let ticks = 0;
  micTimer = setInterval(() => {
    ticks++;
    const wobble = fx.rms <= -120 ? 0 : Math.sin(ticks / 2) * 8 - Math.random() * 4;
    emit("mictest:level", { device: MIC_DEVICE, rmsDb: fx.rms + wobble, peakDb: Math.min(0, fx.peak + wobble) });
    if (ticks >= 15 && screen !== "mictest-running") {
      clearInterval(micTimer);
      emit("mictest:done", { device: MIC_DEVICE, verdict: fx.verdict, message: fx.message, stopped: false });
    }
  }, 100);
}
export async function StopMicTest() {
  if (micTimer === undefined) return;
  clearInterval(micTimer);
  micTimer = undefined;
  emit("mictest:done", { device: MIC_DEVICE, verdict: "", message: "", stopped: true });
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
  "instructions/projects/initech-web.md": "# Project: initech-web\n\n## Key\ninitech-web\n\n## Name\n\n## Aliases\n\n## About\n\n## Existing work\n",
  "instructions/people/ana/initech-web.md": "# ana — initech-web\n\n## Project root\n\n## Tools\n",
  "instructions/people/ana/globex.md": "# ana — globex\n\n## Project root\n\n## Tools\n",
};
const createdFiles = [
  { path: "instructions/projects/initech-web.md", kind: "project", readOnly: false },
  { path: "instructions/people/ana/initech-web.md", kind: "person-project", readOnly: false },
];
const myFileCreatedEntry = { path: "instructions/people/ana/globex.md", kind: "person-project", readOnly: false };
export async function ListInstructionFiles() {
  if (screen === "first-run" || screen === "setup-clone-failed") return [];
  if (screen === "setup-project-created" && projectCreated) return [...instructionFiles, ...createdFiles];
  if (screen === "setup-my-file-created" && myFileCreated) return [...instructionFiles, myFileCreatedEntry];
  return instructionFiles;
}
export async function ListProfiles() {
  return ["ana", "bruno"];
}
export async function ListProfileFiles(name: string) {
  return [`instructions/people/${name}.local.md`, `instructions/people/${name}.md`, `instructions/people/${name}/`, `instructions/people/${name}/acme.md`, `instructions/people/${name}/globex.md`];
}
export async function DeleteProfile(_name: string, _files: string[]) {}
export async function ListMyProjectKeys() {
  if (screen === "setup-project-created" && projectCreated) return ["acme", "initech-web"];
  if (screen === "setup-my-file-created" && myFileCreated) return ["acme", "globex"];
  return ["acme"];
}
export async function CreateMyProjectFile(key: string) {
  myFileCreated = true;
  return `instructions/people/ana/${key}.md`;
}
const NEW_FOLDER = "/Users/ana/Documents/meetings";
const NEW_FOLDER_FILES = ["bin/meeting", "prompts/base.md", ".claude/skills/meetings/SKILL.md", "instructions/README.md", "instructions/people/.gitkeep", "instructions/projects/.gitkeep", "meetings/.gitkeep", ".githooks/pre-commit", ".gitignore", "CLAUDE.md"];
export async function ChooseFolder(_title: string, _startDir: string) {
  return screen.startsWith("setup-folder") ? NEW_FOLDER : ARCHIVE;
}
export async function InspectArchive(path: string) {
  if (screen === "setup-folder-new") return { path, missing: NEW_FOLDER_FILES, inRepo: false, nested: false, repoRoot: "", ready: false };
  if (screen === "setup-folder-in-repo") return { path, missing: NEW_FOLDER_FILES.slice(0, 4).concat(".gitignore (adds the recording rules)"), inRepo: true, nested: true, repoRoot: "/Users/ana/Documents", ready: false };
  return { path, missing: [], inRepo: true, nested: false, repoRoot: path, ready: true };
}
export async function PrepareArchive(_path: string) {}
export async function CloneArchive(_address: string, _parentDir: string) {
  if (screen === "setup-clone-failed") throw new Error("git clone failed:\nERROR: Repository not found.\nfatal: Could not read from remote repository.");
  return "/Users/ana/code/meetings-archive";
}
export async function CreateProfile(_name: string) {
  return "instructions/people/carla.md";
}
export async function CreateProject(_key: string) {
  projectCreated = true;
  return createdFiles.map((f) => f.path);
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
  if (screen === "setup-my-file" || screen === "setup-my-file-created") return ["acme", "globex"];
  if (setupScreens.includes(screen)) return screen === "setup-project-created" && projectCreated ? ["acme", "initech-web"] : ["acme"];
  return projectsEmpty ? [] : projectFixtures;
}
export async function SetMeetingProject(_id: string, _key: string) {}
export async function GenerateSummary(_id: string) {}
export async function ReviewSuggestions(_id: string) {}
export async function GetReviewState(_id: string) {
  return { running: false };
}
export async function CommitAndPushMeeting(_id: string) {
  if (screen === "save-failed") throw new Error("The commit was rejected:\npre-commit hook failed: trailing whitespace in summary.md");
  return { sha: "abc1234", pushed: true };
}

export async function ListMeetings() {
  // The mic test is off while a recording runs, so its screens list none except the one that shows why.
  if (screen.startsWith("mictest") && screen !== "mictest-recording") return meetings.filter((m) => m.state !== "recording");
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
export async function GetDeletePlan(id: string) {
  const m = meetings.find((x) => x.id === id) ?? meetings[0];
  return { audioFiles: 6, archiveFolder: m.folder };
}
export async function DeleteMeeting(_id: string) {
  return { warning: "" };
}
export async function DownloadModels(_language: string) {}
export async function ListOpenForms() {
  const f = fixtureFor(screen);
  return f ? [f] : [];
}
export async function AnswerForm(_formId: string, _answers: Record<string, unknown>, _note: string) {}
export async function DismissForm(_formId: string, _note: string) {}
export async function SetWindowVisible(_visible: boolean) {}
