import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MeetingPage } from "./MeetingPage";
import { GetMeeting, GetRetranscribeStatus, GetTranscript, Retranscribe, emit, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));
vi.mock("@xterm/xterm", () => ({ Terminal: class { cols = 80; rows = 24; open() {} loadAddon() {} attachCustomKeyEventHandler() {} focus() {} write() {} onData() { return { dispose() {} }; } onResize() { return { dispose() {} }; } dispose() {} } }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));
beforeAll(() => { vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} unobserve() {} }); });

beforeEach(resetBindings);

const seg = (id: number, text: string) => ({ id, track: "mic", startMs: id * 1000, endMs: id * 1000 + 900, text });
const button = () => screen.queryByRole("button", { name: "Re-transcribe" });

test("only a ready or summarized meeting offers Re-transcribe", async () => {
  for (const state of ["recording", "transcribing"]) {
    GetMeeting.mockResolvedValue(meeting({ state }));
    const { unmount } = render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    expect(button()).not.toBeInTheDocument();
    unmount();
  }
  for (const state of ["ready", "summarized"]) {
    GetMeeting.mockResolvedValue(meeting({ state }));
    const { unmount } = render(<MeetingPage meetingId="m1" />);
    expect(await screen.findByRole("button", { name: "Re-transcribe" })).toBeEnabled();
    unmount();
  }
});

test("asks first, then shows part n of N while the old transcript stays, then swaps it in", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "summarized" }));
  GetTranscript.mockResolvedValueOnce([seg(1, "old line")]);
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("old line")).toBeInTheDocument();

  fireEvent.click(await screen.findByRole("button", { name: "Re-transcribe" }));
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText("The summary and the suggestions are not changed.")).toBeInTheDocument();
  expect(Retranscribe).not.toHaveBeenCalled();
  await act(async () => {
    fireEvent.click(within(dialog).getByRole("button", { name: "Re-transcribe" }));
  });
  expect(Retranscribe).toHaveBeenCalledWith("m1");
  expect(button()).toBeDisabled();

  act(() => emit("transcript:retranscribe", { meetingId: "m1", state: "running", done: 0, total: 4 }));
  expect(screen.getByText("Transcribing again… part 1 of 4")).toBeInTheDocument();
  act(() => emit("transcript:retranscribe", { meetingId: "m1", state: "running", done: 3, total: 4 }));
  expect(screen.getByText("Transcribing again… part 4 of 4")).toBeInTheDocument();
  expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "3");
  act(() => emit("transcript:retranscribe", { meetingId: "other", state: "done", done: 4, total: 4 }));
  expect(screen.getByText("old line")).toBeInTheDocument();

  GetTranscript.mockResolvedValueOnce([seg(5, "new line")]);
  act(() => emit("transcript:retranscribe", { meetingId: "m1", state: "done", done: 4, total: 4 }));
  expect(await screen.findByText("new line")).toBeInTheDocument();
  expect(screen.queryByText("old line")).not.toBeInTheDocument();
  expect(screen.queryByText(/Transcribing again/)).not.toBeInTheDocument();
  expect(button()).toBeEnabled();
});

test("a failure shows the message and keeps the old transcript", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "ready" }));
  GetTranscript.mockResolvedValue([seg(1, "old line")]);
  render(<MeetingPage meetingId="m1" />);
  fireEvent.click(await screen.findByRole("button", { name: "Re-transcribe" }));
  await act(async () => {
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Re-transcribe" }));
  });
  act(() => emit("transcript:retranscribe", { meetingId: "m1", state: "error", done: 1, total: 2, error: "Transcribing again failed: mic part 2: boom. The old transcript is kept." }));
  expect(screen.getByRole("alert")).toHaveTextContent("Transcribing again failed: mic part 2: boom. The old transcript is kept.");
  expect(screen.getByText("old line")).toBeInTheDocument();
  expect(button()).toBeEnabled();
});

test("a refused start shows the reason", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "ready" }));
  Retranscribe.mockRejectedValue("This meeting has no audio to transcribe.");
  render(<MeetingPage meetingId="m1" />);
  fireEvent.click(await screen.findByRole("button", { name: "Re-transcribe" }));
  await act(async () => {
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Re-transcribe" }));
  });
  expect(screen.getByRole("alert")).toHaveTextContent("This meeting has no audio to transcribe.");
  expect(screen.queryByText(/Transcribing again/)).not.toBeInTheDocument();
});

test("a re-transcription already running shows its progress when the page opens", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "ready" }));
  GetRetranscribeStatus.mockResolvedValue({ running: true, done: 2, total: 6 });
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("Transcribing again… part 3 of 6")).toBeInTheDocument();
  await waitFor(() => expect(button()).toBeDisabled());
});
