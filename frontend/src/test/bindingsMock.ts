import { vi } from "vitest";

type Handler = (...data: any[]) => void;
const handlers = new Map<string, Set<Handler>>();

export const EventsOn = vi.fn((name: string, cb: Handler) => {
  if (!handlers.has(name)) handlers.set(name, new Set());
  handlers.get(name)!.add(cb);
  return () => handlers.get(name)?.delete(cb);
});
export const EventsOff = vi.fn((name: string) => {
  handlers.delete(name);
});

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
export const ListProjects = vi.fn();
export const ListOpenForms = vi.fn(async () => []);
export const AnswerForm = vi.fn(async () => {});
export const DismissForm = vi.fn(async () => {});
export const SetWindowVisible = vi.fn(async () => {});
export const SetMeetingProject = vi.fn();
export const GenerateSummary = vi.fn();
export const ReviewSuggestions = vi.fn(async () => {});
export const GetReviewState = vi.fn(async () => ({ running: false }));
export const GetReviewActWithoutAsking = vi.fn(async () => false);
export const SetReviewActWithoutAsking = vi.fn(async (_on: boolean) => {});
export const CommitAndPushMeeting = vi.fn();
export const GetSettings = vi.fn();
export const SetSettings = vi.fn();
export const ListInstructionFiles = vi.fn();
export const ReadInstructionFile = vi.fn();
export const WriteInstructionFile = vi.fn();
export const PreviewPrompt = vi.fn();
export const ChooseFolder = vi.fn();
export const CloneArchive = vi.fn();
export const InspectArchive = vi.fn();
export const PrepareArchive = vi.fn();
export const ListProfiles = vi.fn();
export const CreateProfile = vi.fn();
export const CreateProject = vi.fn();
export const ListProfileFiles = vi.fn();
export const DeleteProfile = vi.fn();
export const ListMyProjectKeys = vi.fn();
export const CreateMyProjectFile = vi.fn();
export const StartSession = vi.fn();
export const WriteSession = vi.fn();
export const ResizeSession = vi.fn();
export const StopSession = vi.fn();
export const GetUpdateStatus = vi.fn();
export const SetUpdateSource = vi.fn();
export const StartUpdate = vi.fn();
export const GetPermissions = vi.fn();
export const AskPermissions = vi.fn();
export const OpenPermissionSettings = vi.fn();
export const StartMicTest = vi.fn();
export const StopMicTest = vi.fn();
export const GetDeletePlan = vi.fn();
export const GetProjectDeletePlan = vi.fn();
export const DeleteProject = vi.fn();
export const CommitProjectDeletion = vi.fn();
export const DeleteMeeting = vi.fn();
export const GetSetupPrompt = vi.fn();
export const ClipboardSetText = vi.fn();
export const ListThemes = vi.fn();
export const GetSelectedTheme = vi.fn();
export const SelectTheme = vi.fn();
export const ImportVSCodeTheme = vi.fn();
export const DeleteTheme = vi.fn();
export const ExportTheme = vi.fn();

const darkTokens = { bg: "#111214", bar: "#0b0c0d", side: "#0b0c0d", card: "#18191c", fg: "#ecebe7", fg2: "#b6b3ab", fg3: "#8d8a83", "fg3-text": "#9a978f", border: "#2a2b2f", sel: "#2c2416", dot: "#3a3b40", accent: "#f2a93b", "on-accent": "#1b1204", rec: "#ff7b6b", "rec-bg": "#3b1d18", wave: "#a8742a", speaker: "#f2b55a" };
const lightTokens = { bg: "#f4f3f0", bar: "#ebe9e4", side: "#ebe9e4", card: "#fbfaf8", fg: "#1b1a17", fg2: "#4f4b43", fg3: "#736e64", "fg3-text": "#6a655b", border: "#dcd8d0", sel: "#f3e3c6", dot: "#cdc8be", accent: "#9a5600", "on-accent": "#ffffff", rec: "#b8321f", "rec-bg": "#f5e0d9", wave: "#d8a75e", speaker: "#9a5600" };
/** The themes a fresh data folder lists: the three built-ins. */
export const builtinThemes = () => [
  { id: "system", name: "Oscilloscope (match system)", mode: "system", builtin: true, tokens: {} },
  { id: "oscilloscope-dark", name: "Oscilloscope Dark", mode: "dark", builtin: true, tokens: { ...darkTokens } },
  { id: "oscilloscope-light", name: "Oscilloscope Light", mode: "light", builtin: true, tokens: { ...lightTokens } },
];

export function resetBindings() {
  handlers.clear();
  for (const f of [ListMeetings, GetMeeting, GetTranscript, StartRecording, StopRecording, GetModelStatus, DownloadModels, ListProjects, SetMeetingProject, GenerateSummary, ReviewSuggestions, GetReviewState, GetReviewActWithoutAsking, SetReviewActWithoutAsking, CommitAndPushMeeting, GetSettings, SetSettings, ListInstructionFiles, ReadInstructionFile, WriteInstructionFile, PreviewPrompt, ChooseFolder, CloneArchive, InspectArchive, PrepareArchive, ListProfiles, CreateProfile, CreateProject, ListProfileFiles, DeleteProfile, ListMyProjectKeys, CreateMyProjectFile, StartSession, WriteSession, ResizeSession, StopSession, GetUpdateStatus, SetUpdateSource, StartUpdate, GetPermissions, AskPermissions, OpenPermissionSettings, StartMicTest, StopMicTest, GetDeletePlan, GetProjectDeletePlan, DeleteProject, CommitProjectDeletion, DeleteMeeting, GetSetupPrompt, ListOpenForms, AnswerForm, DismissForm, SetWindowVisible, ClipboardSetText, ListThemes, GetSelectedTheme, SelectTheme, ImportVSCodeTheme, DeleteTheme, ExportTheme, EventsOn, EventsOff]) {
    f.mockReset();
  }
  EventsOn.mockImplementation((name: string, cb: Handler) => {
    if (!handlers.has(name)) handlers.set(name, new Set());
    handlers.get(name)!.add(cb);
    return () => handlers.get(name)?.delete(cb);
  });
  EventsOff.mockImplementation((name: string) => {
    handlers.delete(name);
  });
  ListMeetings.mockResolvedValue([]);
  ListOpenForms.mockResolvedValue([]);
  AnswerForm.mockResolvedValue(undefined);
  DismissForm.mockResolvedValue(undefined);
  SetWindowVisible.mockResolvedValue(undefined);
  GetTranscript.mockResolvedValue([]);
  DownloadModels.mockResolvedValue(undefined);
  StopRecording.mockResolvedValue(undefined);
  ListProjects.mockResolvedValue(["acme", "globex"]);
  SetMeetingProject.mockResolvedValue(undefined);
  GenerateSummary.mockResolvedValue(undefined);
  ReviewSuggestions.mockResolvedValue(undefined);
  GetReviewState.mockResolvedValue({ running: false });
  GetReviewActWithoutAsking.mockResolvedValue(false);
  SetReviewActWithoutAsking.mockResolvedValue(undefined);
  CommitAndPushMeeting.mockResolvedValue({ sha: "abc1234", pushed: true });
  GetSettings.mockResolvedValue({ profile: "", checkoutPath: "/archive", projectRoots: {} });
  SetSettings.mockResolvedValue(undefined);
  ListInstructionFiles.mockResolvedValue([]);
  ReadInstructionFile.mockResolvedValue("");
  WriteInstructionFile.mockResolvedValue(undefined);
  PreviewPrompt.mockResolvedValue("");
  ChooseFolder.mockResolvedValue("");
  CloneArchive.mockResolvedValue("/x/meetings-archive");
  InspectArchive.mockImplementation(async (path: string) => ({ path, missing: [], inRepo: true, nested: false, repoRoot: path, ready: true }));
  PrepareArchive.mockResolvedValue(undefined);
  ListProfiles.mockResolvedValue(["ana", "bruno"]);
  CreateProfile.mockResolvedValue("instructions/people/carla.md");
  CreateProject.mockResolvedValue(["instructions/projects/initech-web.md", "instructions/people/ana/initech-web.md"]);
  ListProfileFiles.mockResolvedValue(["instructions/people/bruno.md", "instructions/people/bruno/", "instructions/people/bruno/acme.md"]);
  DeleteProfile.mockResolvedValue(undefined);
  ListMyProjectKeys.mockResolvedValue(["acme", "globex"]);
  CreateMyProjectFile.mockResolvedValue("instructions/people/ana/globex.md");
  StartSession.mockResolvedValue(undefined);
  WriteSession.mockResolvedValue(undefined);
  ResizeSession.mockResolvedValue(undefined);
  StopSession.mockResolvedValue(undefined);
  GetUpdateStatus.mockResolvedValue({ sourcePath: "", installedApp: "/Applications/Phonon.app", signing: false, state: "idle", message: "", lines: [], seq: 0 });
  SetUpdateSource.mockResolvedValue(undefined);
  StartUpdate.mockResolvedValue(undefined);
  GetPermissions.mockResolvedValue({ microphone: "allowed", screen: "allowed" });
  AskPermissions.mockResolvedValue({ microphone: "allowed", screen: "allowed" });
  OpenPermissionSettings.mockResolvedValue(undefined);
  StartMicTest.mockResolvedValue(undefined);
  StopMicTest.mockResolvedValue(undefined);
  GetDeletePlan.mockResolvedValue({ audioFiles: 4, archiveFolder: "" });
  DeleteMeeting.mockResolvedValue({ warning: "" });
  GetProjectDeletePlan.mockImplementation(async (key: string) => ({ key, files: [`instructions/projects/${key}.md`, `instructions/people/ana/${key}.md`, `meetings/${key}/2026/2026-10-01-sync/meeting.md`], folders: [`meetings/${key}/2026/2026-10-01-sync/`, `meetings/${key}/2026/`, `meetings/${key}/`], localMeetings: 2, unsavedMeetings: 1, recording: false }));
  DeleteProject.mockImplementation(async (key: string) => ({ deletedMeetings: 2, trackedRemoved: [`instructions/projects/${key}.md`, `meetings/${key}/2026/2026-10-01-sync/meeting.md`] }));
  CommitProjectDeletion.mockResolvedValue(undefined);
  GetSetupPrompt.mockResolvedValue("SETUP PROMPT TEXT");
  ClipboardSetText.mockResolvedValue(true);
  GetModelStatus.mockResolvedValue({ language: "pt-BR", ready: true, missing: "", downloadBytes: 0, downloading: false });
  ListThemes.mockImplementation(async () => builtinThemes());
  GetSelectedTheme.mockImplementation(async () => builtinThemes()[0]);
  SelectTheme.mockImplementation(async (id: string) => builtinThemes().find((t) => t.id === id) ?? builtinThemes()[0]);
  ImportVSCodeTheme.mockResolvedValue({ id: "", name: "", mode: "", builtin: false, tokens: {} });
  DeleteTheme.mockResolvedValue(undefined);
  ExportTheme.mockResolvedValue("");
}

export function meeting(over: Record<string, unknown> = {}) {
  return {
    id: "m1",
    title: "Weekly sync",
    startedAt: "2026-10-23T14:05:00",
    endedAt: "",
    language: "en",
    projectKey: "acme",
    state: "ready",
    hasSession: false,
    folder: "",
    summaryMd: "",
    suggestionsMd: "",
    ...over,
  };
}
