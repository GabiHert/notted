import { act, fireEvent, render, screen } from "@testing-library/react";
import { NewRecording } from "./NewRecording";
import { StartRecording, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

test("defaults to Portuguese (Brazil) with the placeholder and options", async () => {
  render(<NewRecording onStarted={() => {}} />);
  await act(async () => {});
  expect(screen.getByRole("heading", { name: "New recording" })).toBeInTheDocument();
  expect(screen.getByLabelText("Language")).toHaveValue("pt-BR");
  expect(screen.getByRole("option", { name: "English" })).toHaveValue("en");
  expect(screen.getByPlaceholderText("Meeting title (optional)")).toBeInTheDocument();
});

test("shows Starting… while pending, then opens the meeting", async () => {
  let resolve!: (m: unknown) => void;
  StartRecording.mockReturnValue(new Promise((r) => (resolve = r)));
  const onStarted = vi.fn();
  render(<NewRecording onStarted={onStarted} />);
  await act(async () => {});
  fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Weekly sync" } });
  fireEvent.change(screen.getByLabelText("Language"), { target: { value: "en" } });
  fireEvent.click(screen.getByRole("button", { name: "Start recording" }));
  const pending = screen.getByRole("button", { name: "Starting…" });
  expect(pending).toBeDisabled();
  expect(StartRecording).toHaveBeenCalledWith("Weekly sync", "en");
  await act(async () => resolve(meeting({ id: "m9" })));
  expect(onStarted).toHaveBeenCalledWith(expect.objectContaining({ id: "m9" }));
});

test("shows a rejection verbatim and re-enables the button", async () => {
  StartRecording.mockRejectedValue(new Error("Microphone access is off for Notted."));
  render(<NewRecording onStarted={() => {}} />);
  await act(async () => {});
  fireEvent.click(screen.getByRole("button", { name: "Start recording" }));
  expect(await screen.findByText("Microphone access is off for Notted.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Start recording" })).toBeEnabled();
});
