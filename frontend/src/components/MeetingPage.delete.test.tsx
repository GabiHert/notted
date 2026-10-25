import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MeetingPage } from "./MeetingPage";
import { DeleteMeeting, GetDeletePlan, GetMeeting, emit, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

async function open(over: Record<string, unknown> = {}, onDeleted = vi.fn()) {
  GetMeeting.mockResolvedValue(meeting(over));
  render(<MeetingPage meetingId="m1" onDeleted={onDeleted} />);
  await screen.findByRole("heading", { level: 1, name: "Weekly sync" });
  return onDeleted;
}
const askToDelete = async () => {
  fireEvent.click(screen.getByRole("button", { name: "Delete meeting" }));
  return screen.findByRole("dialog");
};

test("the page has a Delete meeting button and nothing is deleted or asked until it is clicked", async () => {
  await open();
  const button = screen.getByRole("button", { name: "Delete meeting" });
  expect(button).toHaveClass("nt-btn--danger-quiet");
  expect(button).toBeEnabled();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(DeleteMeeting).not.toHaveBeenCalled();
});

test("the confirm names the meeting and lists what goes, without an archive folder", async () => {
  GetDeletePlan.mockResolvedValue({ audioFiles: 4, archiveFolder: "" });
  await open();
  const dialog = await askToDelete();
  expect(GetDeletePlan).toHaveBeenCalledWith("m1");
  expect(within(dialog).getByRole("heading", { name: "Delete “Weekly sync”?" })).toBeInTheDocument();
  expect(within(dialog).getByText("The audio (4 files), the transcript, the summary and the suggestions.")).toBeInTheDocument();
  expect(within(dialog).getByText("This cannot be undone.")).toBeInTheDocument();
  expect(within(dialog).queryByText(/in the archive/)).not.toBeInTheDocument();
  expect(within(dialog).getByRole("button", { name: "Delete meeting" })).toHaveClass("nt-btn--danger");
  expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  expect(DeleteMeeting).not.toHaveBeenCalled();
});

test("one audio file reads in the singular", async () => {
  GetDeletePlan.mockResolvedValue({ audioFiles: 1, archiveFolder: "" });
  await open();
  const dialog = await askToDelete();
  expect(within(dialog).getByText("The audio (1 file), the transcript, the summary and the suggestions.")).toBeInTheDocument();
});

test("a meeting saved to the archive says its folder there is not changed", async () => {
  GetDeletePlan.mockResolvedValue({ audioFiles: 2, archiveFolder: "meetings/acme/2026/2026-10-21-kickoff" });
  await open({ folder: "meetings/acme/2026/2026-10-21-kickoff" });
  const dialog = await askToDelete();
  expect(dialog.querySelector(".nt-dialog__hint")).toHaveTextContent(
    "This cannot be undone. The meeting's folder in the archive (meetings/acme/2026/2026-10-21-kickoff) is not changed.",
  );
});

test("Cancel closes the confirm and deletes nothing", async () => {
  await open();
  const dialog = await askToDelete();
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(DeleteMeeting).not.toHaveBeenCalled();
});

test("Escape cancels too", async () => {
  await open();
  await askToDelete();
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(DeleteMeeting).not.toHaveBeenCalled();
});

test("confirming deletes the meeting and leaves the page", async () => {
  const onDeleted = await open();
  const dialog = await askToDelete();
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Delete meeting" })));
  expect(DeleteMeeting).toHaveBeenCalledWith("m1");
  expect(onDeleted).toHaveBeenCalledTimes(1);
  expect(onDeleted).toHaveBeenCalledWith("");
});

test("a late event for the deleted meeting is ignored", async () => {
  await open();
  const dialog = await askToDelete();
  GetMeeting.mockClear();
  GetMeeting.mockRejectedValue(new Error("Meeting not found."));
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Delete meeting" })));
  await act(async () => emit("recording:state", { meetingId: "m1", state: "ready", error: "Transcribing mic part 2 failed." }));
  await act(async () => emit("transcript:segment", { meetingId: "m1", segment: { id: 9, track: "mic", startMs: 0, endMs: 1, text: "late" } }));
  expect(GetMeeting).not.toHaveBeenCalled();
  expect(screen.queryByText("Transcribing mic part 2 failed.")).not.toBeInTheDocument();
  expect(screen.queryByText("Meeting not found.")).not.toBeInTheDocument();
  expect(screen.queryByText("late")).not.toBeInTheDocument();
});

test("a failed delete closes the confirm, shows the message and stays on the page", async () => {
  DeleteMeeting.mockRejectedValue(new Error("The meeting was not deleted: disk is full"));
  const onDeleted = await open();
  const dialog = await askToDelete();
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Delete meeting" })));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent("The meeting was not deleted: disk is full");
  expect(onDeleted).not.toHaveBeenCalled();
  expect(screen.getByRole("heading", { level: 1, name: "Weekly sync" })).toBeInTheDocument();
});

test("when the plan cannot be read, the error shows and no confirm opens", async () => {
  GetDeletePlan.mockRejectedValue(new Error("Meeting not found."));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "Delete meeting" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Meeting not found.");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("while the meeting records, Delete meeting is disabled and says why", async () => {
  await open({ state: "recording" });
  const button = screen.getByRole("button", { name: "Delete meeting" });
  expect(button).toBeDisabled();
  expect(button).toHaveAttribute("title", "Stop the recording before you delete this meeting.");
  fireEvent.click(button);
  expect(GetDeletePlan).not.toHaveBeenCalled();
});

test("when only the audio folder stays, the page still leaves and passes the warning on", async () => {
  const warning = "The meeting was deleted, but its audio folder could not be removed: permission denied";
  DeleteMeeting.mockResolvedValue({ warning });
  const onDeleted = await open();
  const dialog = await askToDelete();
  GetMeeting.mockClear();
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Delete meeting" })));
  expect(onDeleted).toHaveBeenCalledWith(warning);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  await act(async () => emit("recording:state", { meetingId: "m1", state: "ready", error: "late" }));
  expect(GetMeeting).not.toHaveBeenCalled();
});
