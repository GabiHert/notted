import { act, fireEvent, render, screen } from "@testing-library/react";
import { SaveDialog } from "./SaveDialog";
import { CommitAndPushMeeting, emit, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

const ready = { meetingId: "m1", folder: "meetings/acme/2026/2026-10-23-weekly-sync", files: ["meetings/acme/2026/2026-10-23-weekly-sync/transcript.md", "meetings/acme/2026/2026-10-23-weekly-sync/summary.md"] };

async function open(onDeclined = vi.fn()) {
  render(<SaveDialog meetingId="m1" onDeclined={onDeclined} />);
  await act(async () => emit("export:ready", ready));
  return onDeclined;
}

test("opens only on export:ready for this meeting", async () => {
  render(<SaveDialog meetingId="m1" onDeclined={vi.fn()} />);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await act(async () => emit("export:ready", { ...ready, meetingId: "other" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await act(async () => emit("export:ready", ready));
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Save this meeting to git?" })).toBeInTheDocument();
});

test("lists the folder and the file names", async () => {
  await open();
  expect(screen.getByText("Notted wrote these files to meetings/acme/2026/2026-10-23-weekly-sync in the archive folder:")).toBeInTheDocument();
  expect(screen.getByText("transcript.md")).toBeInTheDocument();
  expect(screen.getByText("summary.md")).toBeInTheDocument();
  expect(screen.getByText("Nothing is staged or committed until you choose.")).toBeInTheDocument();
});

test("Not now closes without a call and reports it", async () => {
  const onDeclined = await open();
  fireEvent.click(screen.getByRole("button", { name: "Not now" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(CommitAndPushMeeting).not.toHaveBeenCalled();
  expect(onDeclined).toHaveBeenCalled();
});

test("Escape is Not now", async () => {
  const onDeclined = await open();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(CommitAndPushMeeting).not.toHaveBeenCalled();
  expect(onDeclined).toHaveBeenCalled();
});

test("Commit and push: pending then success", async () => {
  let resolve!: (v: { sha: string; pushed: boolean }) => void;
  CommitAndPushMeeting.mockReturnValue(new Promise((r) => (resolve = r)));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "Commit and push" }));
  expect(CommitAndPushMeeting).toHaveBeenCalledWith("m1");
  expect(screen.getByRole("button", { name: "Committing…" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled();
  await act(async () => resolve({ sha: "abc1234", pushed: true }));
  expect(screen.getByText("Committed and pushed: abc1234")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("a rejection is shown verbatim with Try again", async () => {
  const msg = "The commit was rejected:\nhook failed\nline two";
  CommitAndPushMeeting.mockRejectedValueOnce(new Error(msg));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "Commit and push" }));
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toBe(msg);
  expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  expect(CommitAndPushMeeting).toHaveBeenCalledTimes(2);
  expect(await screen.findByText("Committed and pushed: abc1234")).toBeInTheDocument();
});

test("focus is trapped inside the dialog", async () => {
  await open();
  const notNow = screen.getByRole("button", { name: "Not now" });
  const commit = screen.getByRole("button", { name: "Commit and push" });
  commit.focus();
  fireEvent.keyDown(commit, { key: "Tab" });
  expect(notNow).toHaveFocus();
  fireEvent.keyDown(notNow, { key: "Tab", shiftKey: true });
  expect(commit).toHaveFocus();
});

test("after success Close has focus and Escape on the page closes the dialog", async () => {
  await open();
  fireEvent.click(screen.getByRole("button", { name: "Commit and push" }));
  const close = await screen.findByRole("button", { name: "Close" });
  expect(close).toHaveFocus();
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("after a failure focus is inside the panel, on Try again", async () => {
  CommitAndPushMeeting.mockRejectedValueOnce(new Error("The commit was rejected:\nno"));
  await open();
  fireEvent.click(screen.getByRole("button", { name: "Commit and push" }));
  const retry = await screen.findByRole("button", { name: "Try again" });
  expect(screen.getByRole("dialog")).toContainElement(document.activeElement as HTMLElement);
  expect(retry).toHaveFocus();
});

test("Tab from the last control wraps to the first, also after success and when focus is on the page", async () => {
  await open();
  fireEvent.click(screen.getByRole("button", { name: "Commit and push" }));
  const close = await screen.findByRole("button", { name: "Close" });
  fireEvent.keyDown(close, { key: "Tab" });
  expect(close).toHaveFocus();
  (document.activeElement as HTMLElement).blur();
  fireEvent.keyDown(document.body, { key: "Tab" });
  expect(close).toHaveFocus();
});

test("a commit with no remote says nothing was pushed", async () => {
  CommitAndPushMeeting.mockResolvedValue({ sha: "abc1234", pushed: false });
  await open();
  fireEvent.click(screen.getByRole("button", { name: "Commit and push" }));
  expect(await screen.findByText("Committed abc1234. This archive has no remote, so nothing was pushed.")).toBeInTheDocument();
  expect(screen.queryByText(/Committed and pushed/)).not.toBeInTheDocument();
});
