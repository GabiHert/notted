import { act, fireEvent, render, screen } from "@testing-library/react";
import { MeetingPage } from "./MeetingPage";
import { GetMeeting, GetTranscript, StopRecording, emit, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

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
