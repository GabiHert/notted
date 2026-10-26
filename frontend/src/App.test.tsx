import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { App } from "./App";
import { DeleteMeeting, GetMeeting, GetSettings, SetWindowVisible, StartRecording, emit, ListInstructionFiles, ListMeetings, ReadInstructionFile, meeting, resetBindings } from "./test/bindingsMock";

vi.mock("./api/bindings", () => import("./test/bindingsMock"));
vi.mock("@uiw/react-codemirror", () => import("./test/codemirrorMock"));

beforeEach(resetBindings);

test("shows the title bar, the sidebar and the new-recording view when there are no meetings", async () => {
  render(<App />);
  expect(screen.getByText("Notted")).toBeInTheDocument();
  expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
});

test("opens the newest meeting on launch", async () => {
  ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" }), meeting({ id: "m1", title: "Older" })]);
  GetMeeting.mockImplementation(async (id: string) => meeting({ id, title: id === "m2" ? "Newest" : "Older" }));
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
});

test("an empty saved archive opens Settings with the Set up Notted card, even when meetings exist", async () => {
  ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" })]);
  GetSettings.mockResolvedValue({ profile: "", checkoutPath: "", projectRoots: {} });
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
  expect(await screen.findByText("Set up Notted")).toBeInTheDocument();
});

test("a GetSettings rejection falls back to the newest meeting", async () => {
  ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" })]);
  GetMeeting.mockResolvedValue(meeting({ id: "m2", title: "Newest" }));
  GetSettings.mockRejectedValue(new Error("no settings"));
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
});

describe("leaving Settings with an unsaved instruction file", () => {
  async function openDirtyFile() {
    ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" })]);
    GetMeeting.mockResolvedValue(meeting({ id: "m2", title: "Newest" }));
    ListInstructionFiles.mockResolvedValue([{ path: "instructions/projects/acme.md", kind: "project", readOnly: false }]);
    ReadInstructionFile.mockResolvedValue("# Acme");
    render(<App initialView={{ kind: "settings" }} />);
    fireEvent.click(await screen.findByRole("button", { name: /^acme\.md/ }));
    fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
    await screen.findByText("Unsaved changes");
  }

  test("choosing a meeting asks first; Keep editing stays, Discard navigates", async () => {
    await openDirtyFile();
    fireEvent.click(await screen.findByRole("button", { name: /Newest/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "Discard unsaved changes?" })).toBeInTheDocument();
    expect(within(dialog).getByText("Your changes to instructions/projects/acme.md are not saved.")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Keep editing" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByLabelText("File contents")).toHaveValue("edited");
    fireEvent.click(screen.getByRole("button", { name: /Newest/ }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Discard" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  test("New recording asks first and Escape keeps editing", async () => {
    await openDirtyFile();
    fireEvent.click(screen.getByRole("button", { name: "New recording" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByLabelText("File contents")).toBeInTheDocument();
  });

  test("a clean file leaves Settings without asking", async () => {
    ListInstructionFiles.mockResolvedValue([]);
    render(<App initialView={{ kind: "settings" }} />);
    await screen.findByLabelText("Profile");
    fireEvent.click(screen.getByRole("button", { name: "New recording" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  });
});

describe("the menu bar", () => {
  test("a failed start shows its message as an error card on the current screen", async () => {
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    act(() => emit("menubar:error", { message: "The speech model is not installed." }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The speech model is not installed.");
    expect(screen.getByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  });

  test("the error goes away when you open another screen", async () => {
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    act(() => emit("menubar:error", { message: "No microphone." }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  test("a recording started from the menu opens its meeting from the New recording screen", async () => {
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    GetMeeting.mockResolvedValue(meeting({ id: "m9", title: "From the menu", state: "recording" }));
    ListMeetings.mockResolvedValue([meeting({ id: "m9", title: "From the menu", state: "recording" })]);
    act(() => emit("recording:state", { meetingId: "m9", state: "recording" }));
    expect(await screen.findByRole("heading", { level: 1, name: "From the menu" })).toBeInTheDocument();
  });

  test("a recording started from the menu leaves Settings alone", async () => {
    ListInstructionFiles.mockResolvedValue([]);
    render(<App initialView={{ kind: "settings" }} />);
    await screen.findByLabelText("Profile");
    act(() => emit("recording:state", { meetingId: "m9", state: "recording" }));
    expect(screen.getByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
  });

  test("a stop does not move the view", async () => {
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    act(() => emit("recording:state", { meetingId: "m9", state: "transcribing" }));
    expect(screen.getByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  });

  test("an error is cleared by the next successful start", async () => {
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    act(() => emit("menubar:error", { message: "No microphone." }));
    await screen.findByRole("alert");
    GetMeeting.mockResolvedValue(meeting({ id: "m9", title: "From the menu", state: "recording" }));
    act(() => emit("recording:state", { meetingId: "m9", state: "recording" }));
    expect(await screen.findByRole("heading", { level: 1, name: "From the menu" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  test("an error is cleared when a recording starts from the window", async () => {
    StartRecording.mockResolvedValue(meeting({ id: "m8", title: "From the window", state: "recording" }));
    GetMeeting.mockResolvedValue(meeting({ id: "m8", title: "From the window", state: "recording" }));
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    act(() => emit("menubar:error", { message: "No microphone." }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: /^Start recording/ }));
    expect(await screen.findByRole("heading", { level: 1, name: "From the window" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  test("a silence warning on New recording does not navigate or clear the error", async () => {
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    act(() => emit("menubar:error", { message: "No microphone." }));
    await screen.findByRole("alert");
    act(() => emit("recording:state", { meetingId: "m9", state: "recording", warning: "No system audio was heard." }));
    expect(screen.getByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("No microphone.");
  });
});

describe("deleting a meeting", () => {
  const open2 = async () => {
    ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" }), meeting({ id: "m1", title: "Older" })]);
    GetMeeting.mockImplementation(async (id: string) => meeting({ id, title: id === "m2" ? "Newest" : "Older" }));
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "Newest" });
  };
  const confirmDelete = async () => {
    fireEvent.click(screen.getByRole("button", { name: "Delete meeting" }));
    const dialog = await screen.findByRole("dialog");
    await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Delete meeting" })));
  };

  test("goes to the next meeting in the list", async () => {
    await open2();
    ListMeetings.mockResolvedValue([meeting({ id: "m1", title: "Older" })]);
    await confirmDelete();
    expect(await screen.findByRole("heading", { level: 1, name: "Older" })).toBeInTheDocument();
  });

  test("goes to New recording when the list is empty", async () => {
    await open2();
    ListMeetings.mockResolvedValue([]);
    await confirmDelete();
    expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  });

  test("goes to New recording when the list cannot be read", async () => {
    await open2();
    ListMeetings.mockRejectedValue(new Error("db closed"));
    await confirmDelete();
    expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  });

  test("the sidebar drops the meeting", async () => {
    await open2();
    ListMeetings.mockResolvedValue([meeting({ id: "m1", title: "Older" })]);
    await confirmDelete();
    await act(async () => emit("meeting:deleted", { meetingId: "m2" }));
    const nav = screen.getByRole("navigation", { name: "Meetings" });
    expect(within(nav).queryByText("Newest")).not.toBeInTheDocument();
    expect(within(nav).getByText("Older")).toBeInTheDocument();
  });

  test("a warning from the delete shows on the page you land on", async () => {
    await open2();
    const warning = "The meeting was deleted, but its audio folder could not be removed: permission denied";
    DeleteMeeting.mockResolvedValue({ warning });
    ListMeetings.mockResolvedValue([meeting({ id: "m1", title: "Older" })]);
    await confirmDelete();
    expect(await screen.findByRole("heading", { level: 1, name: "Older" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(warning);
  });

  test("a clean delete shows no alert", async () => {
    await open2();
    ListMeetings.mockResolvedValue([meeting({ id: "m1", title: "Older" })]);
    await confirmDelete();
    await screen.findByRole("heading", { level: 1, name: "Older" });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("window visibility and notification clicks", () => {
  const setVisibility = (v: "visible" | "hidden") => Object.defineProperty(document, "visibilityState", { value: v, configurable: true });
  afterEach(() => setVisibility("visible"));

  test("reports visibility on mount and on every visibilitychange", async () => {
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "New recording" });
    expect(SetWindowVisible).toHaveBeenCalledWith(true);
    setVisibility("hidden");
    act(() => void document.dispatchEvent(new Event("visibilitychange")));
    expect(SetWindowVisible).toHaveBeenLastCalledWith(false);
    setVisibility("visible");
    act(() => void document.dispatchEvent(new Event("visibilitychange")));
    expect(SetWindowVisible).toHaveBeenLastCalledWith(true);
  });

  test("a rejected SetWindowVisible is ignored", async () => {
    SetWindowVisible.mockRejectedValue(new Error("no"));
    render(<App />);
    expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  });

  test("ask:open opens that meeting", async () => {
    ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" }), meeting({ id: "m1", title: "Older" })]);
    GetMeeting.mockImplementation(async (id: string) => meeting({ id, title: id === "m2" ? "Newest" : "Older" }));
    render(<App />);
    await screen.findByRole("heading", { level: 1, name: "Newest" });
    await act(async () => emit("ask:open", { meetingId: "m1" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Older" })).toBeInTheDocument();
  });

  test("ask:open with unsaved instructions shows the discard confirm", async () => {
    ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" })]);
    GetMeeting.mockResolvedValue(meeting({ id: "m2", title: "Newest" }));
    ListInstructionFiles.mockResolvedValue([{ path: "instructions/projects/acme.md", kind: "project", readOnly: false }]);
    ReadInstructionFile.mockResolvedValue("# Acme");
    render(<App initialView={{ kind: "settings" }} />);
    fireEvent.click(await screen.findByRole("button", { name: /^acme\.md/ }));
    fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
    await screen.findByText("Unsaved changes");
    await act(async () => emit("ask:open", { meetingId: "m2" }));
    expect(within(screen.getByRole("dialog")).getByRole("heading", { name: "Discard unsaved changes?" })).toBeInTheDocument();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Discard" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
  });
});
