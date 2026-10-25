import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { UpdateCard } from "./UpdateCard";
import { ChooseFolder, GetUpdateStatus, SetUpdateSource, StartUpdate, emit, listenerCount, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

const SOURCE = "/Users/ana/code/notted";
const status = (over: Record<string, unknown> = {}) => ({ sourcePath: SOURCE, installedApp: "/Applications/Notted.app", signing: false, state: "idle", message: "", lines: [] as string[], seq: 0, ...over });

const updateBtn = () => screen.getByRole("button", { name: /^(Update Notted|Updating…)$/ });
const chooseBtn = () => screen.getByRole("button", { name: "Choose the Notted source folder" });
const out = () => screen.getByTestId("update-output");

async function loaded(over: Record<string, unknown> = {}) {
  GetUpdateStatus.mockResolvedValue(status(over));
  const r = render(<UpdateCard />);
  await screen.findByText(/Notted replaces|not running from an app bundle/);
  return r;
}

beforeEach(() => {
  resetBindings();
});

test("renders the header, help, label, empty read-only input, placeholder and Choose…", async () => {
  await loaded({ sourcePath: "" });
  expect(screen.getByText("Update Notted", { selector: ".nt-card__header" })).toBeInTheDocument();
  expect(screen.getByText("Notted pulls the main branch of your Notted source folder, builds it, and reopens as the new version. Nothing runs until you click Update Notted.")).toBeInTheDocument();
  const input = screen.getByLabelText("Notted source folder");
  expect(input).toHaveValue("");
  expect(input).toHaveAttribute("readonly");
  expect(input).toHaveAttribute("placeholder", "Choose the folder of your Notted clone");
  expect(chooseBtn()).toHaveTextContent("Choose…");
  expect(screen.getByText(/^No "Notted Local Signing" certificate in your keychain, so builds are signed ad hoc and macOS asks for the microphone and screen recording permissions again after each update\. To stop that, run bash scripts\/create-signing-identity\.sh once in your Notted source folder\.$/)).toBeInTheDocument();
  expect(screen.getByText("Notted replaces /Applications/Notted.app and reopens it.")).toBeInTheDocument();
  expect(updateBtn()).toBeDisabled();
});

test("signing present shows its line", async () => {
  await loaded({ signing: true });
  expect(screen.getByText('Builds are signed with your "Notted Local Signing" certificate, so macOS keeps the microphone and screen recording permissions after an update.')).toBeInTheDocument();
  expect(updateBtn()).toBeEnabled();
});

test("outside an app bundle: the not-in-a-bundle line and a disabled button", async () => {
  await loaded({ installedApp: "" });
  expect(screen.getByText("Notted is not running from an app bundle, so it cannot update itself.")).toBeInTheDocument();
  expect(updateBtn()).toBeDisabled();
});

test("nothing about the app or signing shows before the status loads, and the buttons are disabled", () => {
  GetUpdateStatus.mockReturnValue(new Promise(() => {}));
  render(<UpdateCard />);
  expect(screen.queryByText(/Notted replaces/)).not.toBeInTheDocument();
  expect(updateBtn()).toBeDisabled();
  expect(chooseBtn()).toBeDisabled();
});

test("Choose… passes the title and the current source, then saves and reloads", async () => {
  await loaded({ sourcePath: "" });
  ChooseFolder.mockResolvedValue(SOURCE);
  GetUpdateStatus.mockResolvedValue(status());
  fireEvent.click(chooseBtn());
  await waitFor(() => expect(screen.getByLabelText("Notted source folder")).toHaveValue(SOURCE));
  expect(ChooseFolder).toHaveBeenCalledWith("Choose the Notted source folder", "");
  expect(SetUpdateSource).toHaveBeenCalledWith(SOURCE);
  expect(GetUpdateStatus).toHaveBeenCalledTimes(2);
  expect(chooseBtn()).toBeEnabled();
});

test("a cancelled picker does nothing", async () => {
  await loaded({ sourcePath: "" });
  fireEvent.click(chooseBtn());
  await waitFor(() => expect(ChooseFolder).toHaveBeenCalled());
  await waitFor(() => expect(chooseBtn()).toBeEnabled());
  expect(SetUpdateSource).not.toHaveBeenCalled();
  expect(GetUpdateStatus).toHaveBeenCalledTimes(1);
});

test("a SetUpdateSource rejection shows verbatim and re-enables Choose…", async () => {
  await loaded({ sourcePath: "" });
  ChooseFolder.mockResolvedValue("/x");
  SetUpdateSource.mockRejectedValue(new Error("That folder is not a Notted source clone: Makefile is missing."));
  fireEvent.click(chooseBtn());
  expect((await screen.findByRole("alert")).textContent).toBe("That folder is not a Notted source clone: Makefile is missing.");
  expect(chooseBtn()).toBeEnabled();
});

test("a ChooseFolder rejection shows verbatim and re-enables Choose…", async () => {
  await loaded();
  ChooseFolder.mockRejectedValue(new Error("The folder picker failed."));
  fireEvent.click(chooseBtn());
  expect((await screen.findByRole("alert")).textContent).toBe("The folder picker failed.");
  expect(chooseBtn()).toBeEnabled();
});

test("a GetUpdateStatus rejection shows verbatim", async () => {
  GetUpdateStatus.mockRejectedValue(new Error("Cannot read the update status."));
  render(<UpdateCard />);
  expect((await screen.findByRole("alert")).textContent).toBe("Cannot read the update status.");
});

test("Update Notted calls StartUpdate once", async () => {
  await loaded({ signing: true });
  fireEvent.click(updateBtn());
  await waitFor(() => expect(StartUpdate).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(GetUpdateStatus).toHaveBeenCalledTimes(2));
});

test("a StartUpdate rejection shows verbatim, the button is enabled again and nothing else changes", async () => {
  await loaded();
  const msg = "The Notted source folder is on feature-x, not main. Switch it to main in Terminal, then update again.";
  StartUpdate.mockRejectedValue(new Error(msg));
  fireEvent.click(updateBtn());
  expect((await screen.findByRole("alert")).textContent).toBe(msg);
  await waitFor(() => expect(updateBtn()).toBeEnabled());
  expect(updateBtn()).toHaveTextContent("Update Notted");
  expect(chooseBtn()).toBeEnabled();
  expect(screen.getByLabelText("Notted source folder")).toHaveValue(SOURCE);
  expect(screen.queryByTestId("update-output")).not.toBeInTheDocument();
});

test("state and output events drive the card", async () => {
  await loaded();
  act(() => emit("update:state", { state: "pulling", message: "" }));
  expect(updateBtn()).toHaveTextContent("Updating…");
  expect(updateBtn()).toBeDisabled();
  expect(chooseBtn()).toBeDisabled();
  expect(document.querySelector(".nt-bar--indeterminate")).toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("Pulling main…");

  act(() => {
    emit("update:output", { seq: 1, line: "one" });
    emit("update:output", { seq: 2, line: "two" });
    emit("update:output", { seq: 3, line: "three" });
  });
  expect(out().textContent).toBe("one\ntwo\nthree");
  act(() => emit("update:output", { seq: 2, line: "two again" }));
  expect(out().textContent).toBe("one\ntwo\nthree");

  act(() => emit("update:state", { state: "building", message: "" }));
  expect(screen.getByRole("status")).toHaveTextContent("Building… This takes a few minutes.");

  const failed = "make build failed. Notted did not change. The output below shows why.";
  act(() => emit("update:state", { state: "failed", message: failed }));
  expect(screen.getByRole("alert").textContent).toBe(failed);
  expect(out().textContent).toBe("one\ntwo\nthree");
  expect(updateBtn()).toBeEnabled();
  expect(updateBtn()).toHaveTextContent("Update Notted");
  expect(document.querySelector(".nt-bar--indeterminate")).not.toBeInTheDocument();

  act(() => emit("update:state", { state: "pulling", message: "" }));
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(out().textContent).toBe("");
});

test("a stale line after a new run starts is dropped by seq, and a late line after failure is not shown twice", async () => {
  await loaded();
  act(() => emit("update:state", { state: "pulling", message: "" }));
  act(() => emit("update:output", { seq: 5, line: "a" }));
  act(() => emit("update:state", { state: "failed", message: "boom" }));
  act(() => emit("update:output", { seq: 5, line: "a" }));
  act(() => emit("update:output", { seq: 4, line: "old" }));
  expect(out().textContent).toBe("a");
});

test("a malformed output event is ignored", async () => {
  await loaded();
  act(() => emit("update:state", { state: "pulling", message: "" }));
  act(() => emit("update:output", undefined));
  expect(out().textContent).toBe("");
});

test("installing shows its message in the status line with both buttons disabled", async () => {
  await loaded();
  const m = "Notted quits now and reopens when the new version is in place.";
  act(() => emit("update:state", { state: "installing", message: m }));
  expect(screen.getByRole("status")).toHaveTextContent(m);
  expect(updateBtn()).toBeDisabled();
  expect(chooseBtn()).toBeDisabled();
});

test("mounting during a run restores lines and state; seq 7 is ignored and seq 8 appends", async () => {
  await loaded({ state: "building", lines: ["a", "b"], seq: 7 });
  expect(out().textContent).toBe("a\nb");
  expect(updateBtn()).toHaveTextContent("Updating…");
  expect(screen.getByRole("status")).toHaveTextContent("Building… This takes a few minutes.");
  act(() => emit("update:output", { seq: 7, line: "dup" }));
  expect(out().textContent).toBe("a\nb");
  act(() => emit("update:output", { seq: 8, line: "c" }));
  expect(out().textContent).toBe("a\nb\nc");
});

test("an event that arrives before the status resolves is kept, and the status lines it already holds are not doubled", async () => {
  let resolve!: (v: unknown) => void;
  GetUpdateStatus.mockReturnValue(new Promise((r) => (resolve = r)));
  render(<UpdateCard />);
  act(() => emit("update:output", { seq: 3, line: "c" }));
  await act(async () => resolve(status({ state: "building", lines: ["a", "b", "c"], seq: 3 })));
  expect(out().textContent).toBe("a\nb\nc");
});

test("5005 output events keep 5000 lines, the first being line 6", async () => {
  await loaded();
  act(() => emit("update:state", { state: "pulling", message: "" }));
  act(() => {
    for (let i = 1; i <= 5005; i++) emit("update:output", { seq: i, line: `line ${i}` });
  });
  const lines = out().textContent!.split("\n");
  expect(lines).toHaveLength(5000);
  expect(lines[0]).toBe("line 6");
  expect(lines[4999]).toBe("line 5005");
});

test("after StartUpdate resolves and pulling arrives, focus is on the output block", async () => {
  await loaded({ signing: true });
  fireEvent.click(updateBtn());
  await waitFor(() => expect(StartUpdate).toHaveBeenCalled());
  GetUpdateStatus.mockResolvedValue(status({ state: "pulling" }));
  act(() => emit("update:state", { state: "pulling", message: "" }));
  await waitFor(() => expect(out()).toHaveFocus());
});

test("a status load that resolves after unmount is dropped", async () => {
  let resolve!: (v: unknown) => void;
  GetUpdateStatus.mockReturnValue(new Promise((r) => (resolve = r)));
  const { unmount } = render(<UpdateCard />);
  unmount();
  await act(async () => resolve(status()));
});

test("unmount removes both listeners", async () => {
  const { unmount } = await loaded();
  expect(listenerCount("update:output")).toBe(1);
  expect(listenerCount("update:state")).toBe(1);
  unmount();
  expect(listenerCount("update:output")).toBe(0);
  expect(listenerCount("update:state")).toBe(0);
});

test("a line that beats an older status answer is merged after the status lines", async () => {
  let resolve!: (v: unknown) => void;
  GetUpdateStatus.mockReturnValue(new Promise((r) => (resolve = r)));
  render(<UpdateCard />);
  act(() => emit("update:output", { seq: 8, line: "c" }));
  await act(async () => resolve(status({ state: "building", lines: ["a", "b"], seq: 7 })));
  expect(out().textContent).toBe("a\nb\nc");
  act(() => emit("update:output", { seq: 9, line: "d" }));
  expect(out().textContent).toBe("a\nb\nc\nd");
});

test("a pulling event during the status call means the status lines are from the previous run and are not used", async () => {
  let resolve!: (v: unknown) => void;
  GetUpdateStatus.mockReturnValue(new Promise((r) => (resolve = r)));
  render(<UpdateCard />);
  act(() => emit("update:state", { state: "pulling", message: "" }));
  act(() => emit("update:output", { seq: 8, line: "new" }));
  await act(async () => resolve(status({ state: "failed", message: "old failure", lines: ["a", "b"], seq: 7 })));
  expect(out().textContent).toBe("new");
  expect(screen.getByRole("status")).toHaveTextContent("Pulling main…");
});

test("the merge respects the 5000 line cap", async () => {
  let resolve!: (v: unknown) => void;
  GetUpdateStatus.mockReturnValue(new Promise((r) => (resolve = r)));
  render(<UpdateCard />);
  act(() => emit("update:output", { seq: 5001, line: "x1" }));
  act(() => emit("update:output", { seq: 5002, line: "x2" }));
  const old = Array.from({ length: 5000 }, (_, i) => `s${i + 1}`);
  await act(async () => resolve(status({ state: "building", lines: old, seq: 5000 })));
  const lines = out().textContent!.split("\n");
  expect(lines).toHaveLength(5000);
  expect(lines[0]).toBe("s3");
  expect(lines.slice(-2)).toEqual(["x1", "x2"]);
});

test("the output block is a named region and is not a live region", async () => {
  await loaded({ state: "building", lines: ["a"], seq: 1 });
  expect(screen.getByRole("region", { name: "Update output" })).toBe(out());
  expect(out()).not.toHaveAttribute("aria-live");
});
