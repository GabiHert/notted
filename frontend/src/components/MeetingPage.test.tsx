import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MeetingPage } from "./MeetingPage";
import { GetMeeting, GetReviewState, GetTranscript, ListOpenForms, ReviewSuggestions, StopRecording, emit, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));
vi.mock("@xterm/xterm", () => ({ Terminal: class { cols = 80; rows = 24; open() {} loadAddon() {} attachCustomKeyEventHandler() {} focus() {} write() {} onData() { return { dispose() {} }; } onResize() { return { dispose() {} }; } dispose() {} } }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));
beforeAll(() => { vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} unobserve() {} }); });

beforeEach(() => {
  resetBindings();
});
afterEach(() => vi.useRealTimers());

const seg = (id: number, track: string, startMs: number, text: string) => ({ id, track, startMs, endMs: startMs + 2000, text });

test("recording: pill ticks from startedAt and Stop recording stops", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  vi.setSystemTime(new Date("2026-10-23T14:05:05"));
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  await act(async () => {});
  expect(screen.getByText("Recording 00:05")).toBeInTheDocument();
  await act(async () => {
    vi.advanceTimersByTime(3000);
  });
  expect(screen.getByText("Recording 00:08")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Stop recording" }));
  expect(StopRecording).toHaveBeenCalledWith("m1");
});

test("transcribing shows the text and no stop button", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "transcribing" }));
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("Transcribing…")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Stop recording" })).not.toBeInTheDocument();
});

test("ready shows title and meta only", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByRole("heading", { level: 1, name: "Weekly sync" })).toBeInTheDocument();
  expect(screen.getByText("Oct 23, 2026, 14:05 · English")).toBeInTheDocument();
  expect(screen.queryByText("Transcribing…")).not.toBeInTheDocument();
});

test("warning and error cards show the message verbatim", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  await screen.findByRole("heading", { level: 1 });
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: "No sound from other people yet." }));
  expect(screen.getByText("Audio")).toBeInTheDocument();
  expect(screen.getByText("No sound from other people yet.")).toBeInTheDocument();
  await act(async () => emit("recording:state", { meetingId: "m1", state: "ready", error: "The recorder stopped." }));
  expect(screen.getByText("The recorder stopped.")).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent("The recorder stopped.");
});

test("a microphone warning and a system audio warning show together, each once", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  await screen.findByRole("heading", { level: 1 });
  const mic = "Your microphone recorded no sound (Studio Mic). If your MacBook's lid is closed, its built-in microphone is off.";
  const system = "No system audio was heard in the last minute.";
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: mic }));
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: system }));
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: mic }));
  expect(screen.getByText(mic)).toBeInTheDocument();
  expect(screen.getByText(system)).toBeInTheDocument();
  expect(screen.getAllByText(mic)).toHaveLength(1);
});

test("ignores state events of other meetings", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<MeetingPage meetingId="m1" />);
  await screen.findByRole("heading", { level: 1 });
  await act(async () => emit("recording:state", { meetingId: "other", state: "ready", error: "Not mine." }));
  expect(screen.queryByText("Not mine.")).not.toBeInTheDocument();
});

test("transcript: stored lines plus a live segment, deduped by id, with speaker labels", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  GetTranscript.mockResolvedValue([seg(1, "mic", 3000, "Hello everyone"), seg(2, "others", 65000, "Good morning")]);
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("Hello everyone")).toBeInTheDocument();
  expect(screen.getByText("00:00:03")).toBeInTheDocument();
  expect(screen.getByText("00:01:05")).toBeInTheDocument();
  expect(screen.getByText("You")).toBeInTheDocument();
  expect(screen.getByText("Others")).toBeInTheDocument();
  await act(async () => emit("transcript:segment", { meetingId: "m1", segment: seg(3, "mic", 70000, "Let us start") }));
  await act(async () => emit("transcript:segment", { meetingId: "m1", segment: seg(3, "mic", 70000, "Let us start") }));
  await act(async () => emit("transcript:segment", { meetingId: "other", segment: seg(9, "mic", 1000, "Elsewhere") }));
  expect(screen.getAllByText("Let us start")).toHaveLength(1);
  expect(screen.queryByText("Elsewhere")).not.toBeInTheDocument();
});

test("empty transcript text", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("The transcript appears here as each 30-second part is transcribed.")).toBeInTheDocument();
});

test("shows the model banner when the language's models are missing", async () => {
  const { GetModelStatus } = await import("../test/bindingsMock");
  GetModelStatus.mockResolvedValue({ language: "en", ready: false, missing: "x", downloadBytes: 100_000_000, downloading: false });
  GetMeeting.mockResolvedValue(meeting({ state: "transcribing" }));
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("Speech models")).toBeInTheDocument();
});

test("an open form shows the card under the header and leaves when the list empties", async () => {
  GetMeeting.mockResolvedValue(meeting());
  const f = { id: "f1", meetingId: "m1", kind: "ask", title: "Where next?", queued: 0, pieces: [{ kind: "text", id: "t", markdown: "hi", html: "", question: "", options: [], placeholder: "", multiline: false, required: false, yesLabel: "Yes", noLabel: "No" }] };
  ListOpenForms.mockResolvedValue([f] as any);
  const { container } = render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByRole("region", { name: "Waiting for you" })).toBeInTheDocument();
  const head = container.querySelector("header")!;
  expect(head.nextElementSibling).toBe(screen.getByRole("region", { name: "Waiting for you" }));
  ListOpenForms.mockResolvedValue([]);
  await act(async () => emit("ask:changed", { meetingId: "m1" }));
  expect(screen.queryByRole("region", { name: "Waiting for you" })).not.toBeInTheDocument();
});

describe("review suggestions", () => {
  const withSuggestions = (over: Record<string, unknown> = {}) => meeting({ state: "summarized", hasSession: true, suggestionsMd: "# Suggestions\n\n- [ ] Send the draft", ...over });

  test("a review already running at load disables the button", async () => {
    GetReviewState.mockResolvedValue({ running: true });
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    expect(await screen.findByText("Reviewing suggestions… Claude Code asks here when it needs you.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled();
    expect(GetReviewState).toHaveBeenCalledWith("m1");
  });

  test("a done event reloads the meeting once and shows the reply", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("button", { name: "Review suggestions" });
    const before = GetMeeting.mock.calls.length;
    await act(async () => emit("review:state", { meetingId: "m1", state: "done", message: "S-01 accepted" }));
    expect(await screen.findByText("Review finished.")).toBeInTheDocument();
    expect(screen.getByText("S-01 accepted")).toBeInTheDocument();
    expect(GetMeeting.mock.calls.length).toBe(before + 1);
  });

  test("an event for another meeting is ignored", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("button", { name: "Review suggestions" });
    await act(async () => emit("review:state", { meetingId: "other", state: "running" }));
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
  });

  test("clicking runs the review; a refusal shows its message", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    ReviewSuggestions.mockRejectedValue(new Error("Close the Claude Code session on this page first."));
    render(<MeetingPage meetingId="m1" />);
    fireEvent.click(await screen.findByRole("button", { name: "Review suggestions" }));
    expect(ReviewSuggestions).toHaveBeenCalledWith("m1");
    expect(await screen.findByText("Close the Claude Code session on this page first.")).toBeInTheDocument();
  });

  test("an open session disables the button, and closing it frees the button", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    const open = await screen.findByRole("button", { name: "Open session" });
    await waitFor(() => expect(open).toBeEnabled());
    fireEvent.click(open);
    await waitFor(() => expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled());
    expect(screen.getByText("Close the Claude Code session below first. The review and the session use the same conversation.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close session" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled());
  });

  test("a running review disables Open session with its reason", async () => {
    GetReviewState.mockResolvedValue({ running: true });
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    expect(await screen.findByText("Suggestions are being reviewed for this meeting. Open the session when the review finishes.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open session" })).toBeDisabled();
  });
});
