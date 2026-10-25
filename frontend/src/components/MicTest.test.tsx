import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MicTest } from "./MicTest";
import { ListMeetings, StartMicTest, StopMicTest, emit, listenerCount, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

const SILENT = "No sound from Studio Mic. If your MacBook's lid is closed, its built-in microphone is off: open the lid, or choose another input in System Settings > Sound > Input.";
const QUIET = "Very quiet. Speak closer to Studio Mic, or choose another input in System Settings > Sound > Input.";
const WORKS = "The microphone works.";

const startButton = () => screen.getByRole("button", { name: "Test microphone" });

async function idle() {
  const r = render(<MicTest />);
  await waitFor(() => expect(ListMeetings).toHaveBeenCalled());
  return r;
}
async function running() {
  const r = await idle();
  fireEvent.click(startButton());
  await screen.findByRole("button", { name: "Stop" });
  return r;
}
const done = (over: Record<string, unknown> = {}) =>
  act(async () => emit("mictest:done", { device: "Studio Mic", verdict: "ok", message: WORKS, stopped: false, ...over }));

beforeEach(() => {
  resetBindings();
});

test("starts idle with the test button and no meter", async () => {
  await idle();
  expect(startButton()).toBeEnabled();
  expect(screen.queryByRole("meter")).not.toBeInTheDocument();
  expect(StartMicTest).not.toHaveBeenCalled();
});

test("Test microphone starts the test and shows the meter and Stop", async () => {
  await running();
  expect(StartMicTest).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("meter")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Test microphone" })).not.toBeInTheDocument();
});

test("the meter follows the level and the device name shows", async () => {
  await running();
  await act(async () => emit("mictest:level", { device: "Studio Mic", rmsDb: -30, peakDb: -12 }));
  expect(screen.getByRole("meter")).toHaveAttribute("aria-valuenow", "-30");
  expect(screen.getByText("Listening to Studio Mic")).toBeInTheDocument();
  await act(async () => emit("mictest:level", { device: "Studio Mic", rmsDb: -120, peakDb: -120 }));
  expect(screen.getByRole("meter")).toHaveAttribute("aria-valuenow", "-60");
  await act(async () => emit("mictest:level", { device: "Studio Mic", rmsDb: -3, peakDb: 0 }));
  expect(screen.getByRole("meter")).toHaveAttribute("aria-valuenow", "-3");
});

test.each([
  ["silent", SILENT],
  ["quiet", QUIET],
  ["ok", WORKS],
])("the %s verdict shows its message and the test can run again", async (verdict, message) => {
  await running();
  await done({ verdict, message });
  const status = screen.getByRole("status");
  expect(status).toHaveTextContent(message);
  expect(status).toHaveClass(`nt-mictest__verdict--${verdict}`);
  expect(screen.queryByRole("meter")).not.toBeInTheDocument();
  expect(startButton()).toBeEnabled();
});

test("a new test clears the last verdict", async () => {
  await running();
  await done({ verdict: "silent", message: SILENT });
  fireEvent.click(startButton());
  await screen.findByRole("button", { name: "Stop" });
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

test("Stop asks the app to stop and a stopped test shows no verdict", async () => {
  await running();
  fireEvent.click(screen.getByRole("button", { name: "Stop" }));
  expect(StopMicTest).toHaveBeenCalledTimes(1);
  await done({ verdict: "", message: "", stopped: true });
  expect(screen.getByRole("status")).toHaveTextContent("Test stopped.");
  expect(screen.getByRole("status")).not.toHaveClass("nt-mictest__verdict--ok");
  expect(startButton()).toBeEnabled();
});

test("the test is disabled while a recording runs and says why", async () => {
  ListMeetings.mockResolvedValue([meeting({ state: "recording" })]);
  await idle();
  await waitFor(() => expect(startButton()).toBeDisabled());
  expect(screen.getByText("Stop the recording to test the microphone.")).toBeInTheDocument();
  expect(StartMicTest).not.toHaveBeenCalled();
});

test("the test turns on again when the recording ends", async () => {
  ListMeetings.mockResolvedValue([meeting({ state: "recording" })]);
  await idle();
  await waitFor(() => expect(startButton()).toBeDisabled());
  ListMeetings.mockResolvedValue([meeting({ state: "transcribing" })]);
  await act(async () => emit("recording:state", { meetingId: "m1", state: "transcribing" }));
  await waitFor(() => expect(startButton()).toBeEnabled());
  expect(screen.queryByText("Stop the recording to test the microphone.")).not.toBeInTheDocument();
});

test("a refused start shows the app's message, such as the microphone permission", async () => {
  StartMicTest.mockRejectedValue(new Error("Microphone access is off for Notted. Allow it in the Permissions card (Ask, or Open System Settings), then test again."));
  await idle();
  fireEvent.click(startButton());
  expect(await screen.findByRole("alert")).toHaveTextContent("Microphone access is off for Notted.");
  expect(screen.queryByRole("meter")).not.toBeInTheDocument();
  expect(startButton()).toBeEnabled();
});

test("a helper failure shows its message and ends the test", async () => {
  await running();
  await act(async () => emit("mictest:error", { message: "The microphone test stopped unexpectedly. Try again." }));
  expect(screen.getByRole("alert")).toHaveTextContent("The microphone test stopped unexpectedly. Try again.");
  expect(screen.queryByRole("meter")).not.toBeInTheDocument();
  expect(startButton()).toBeEnabled();
});

test("an old error disappears when the next test starts", async () => {
  await running();
  await act(async () => emit("mictest:error", { message: "The microphone test stopped unexpectedly. Try again." }));
  fireEvent.click(startButton());
  await screen.findByRole("button", { name: "Stop" });
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

test("a level that arrives after the test ended is ignored", async () => {
  await running();
  await done();
  await act(async () => emit("mictest:level", { device: "Studio Mic", rmsDb: -20, peakDb: -10 }));
  expect(screen.queryByRole("meter")).not.toBeInTheDocument();
});

test("leaving while a test runs stops it, and listeners are removed", async () => {
  const { unmount } = await running();
  unmount();
  expect(StopMicTest).toHaveBeenCalledTimes(1);
  expect(listenerCount("mictest:level")).toBe(0);
  expect(listenerCount("mictest:done")).toBe(0);
  expect(listenerCount("mictest:error")).toBe(0);
  expect(listenerCount("recording:state")).toBe(0);
});

test("leaving while idle does not call Stop", async () => {
  const { unmount } = await idle();
  unmount();
  expect(StopMicTest).not.toHaveBeenCalled();
});
