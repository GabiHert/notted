import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { InstructionFiles } from "./InstructionFiles";
import { ListInstructionFiles, ReadInstructionFile, WriteInstructionFile, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));
vi.mock("@uiw/react-codemirror", () => import("../test/codemirrorMock"));

const FILES = [
  { path: "prompts/base.md", kind: "base", readOnly: true },
  { path: "instructions/README.md", kind: "readme", readOnly: false },
  { path: "instructions/projects/acme.md", kind: "project", readOnly: false },
  { path: "instructions/people/ana.md", kind: "person", readOnly: false },
  { path: "instructions/people/ana/acme.md", kind: "person-project", readOnly: false },
  { path: "instructions/local/notes.md", kind: "local", readOnly: false },
];
const TEXT: Record<string, string> = {
  "prompts/base.md": "# Base prompt\n\nAlways be brief.",
  "instructions/README.md": "# Guide",
  "instructions/projects/acme.md": "# Acme\n\nUse the acme glossary.",
  "instructions/people/ana.md": "# Ana",
};

beforeEach(() => {
  resetBindings();
  ListInstructionFiles.mockResolvedValue(FILES);
  ReadInstructionFile.mockImplementation(async (p: string) => TEXT[p] ?? "");
});

const open = async (name: string) => {
  fireEvent.click(await screen.findByRole("button", { name: new RegExp(name) }));
};

test("groups the files under their labels in the fixed order", async () => {
  render(<InstructionFiles />);
  await screen.findByText("Base prompt");
  const labels = [...document.querySelectorAll(".nt-files__group-label")].map((e) => e.textContent);
  expect(labels).toEqual(["Base prompt", "Guide", "Projects", "People", "Per project", "Local overrides"]);
  expect(screen.getByText("Instruction files")).toBeInTheDocument();
  expect(screen.getByText("Choose a file to read or edit it.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /ana\/acme\.md/ })).toBeInTheDocument();
  expect(within(screen.getByRole("button", { name: /base\.md/ })).getByText("Read-only")).toBeInTheDocument();
  expect(within(screen.getByRole("button", { name: /^acme\.md/ })).queryByText("Read-only")).not.toBeInTheDocument();
});

test("opening a file reads it and shows the editor with Edit and Preview", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  expect(ReadInstructionFile).toHaveBeenCalledWith("instructions/projects/acme.md");
  expect(await screen.findByLabelText("File contents")).toHaveValue("# Acme\n\nUse the acme glossary.");
  expect(screen.getByText("instructions/projects/acme.md")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(screen.getByRole("heading", { name: "Acme" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  expect(screen.getByLabelText("File contents")).toBeInTheDocument();
});

test("editing marks Unsaved changes; Save writes the text and shows Saved.", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  const box = await screen.findByLabelText("File contents");
  fireEvent.change(box, { target: { value: "# Acme\n\nNew rule." } });
  expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByText("Saved.")).toBeInTheDocument();
  expect(WriteInstructionFile).toHaveBeenCalledWith("instructions/projects/acme.md", "# Acme\n\nNew rule.");
  expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
});

test("Mod-s saves like the button", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  const box = await screen.findByLabelText("File contents");
  fireEvent.change(box, { target: { value: "changed" } });
  fireEvent.keyDown(box, { key: "s", ctrlKey: true });
  expect(await screen.findByText("Saved.")).toBeInTheDocument();
  expect(WriteInstructionFile).toHaveBeenCalledWith("instructions/projects/acme.md", "changed");
});

test("a rejected save is shown verbatim and the file stays dirty", async () => {
  WriteInstructionFile.mockRejectedValue(new Error("This file is read-only in Notted."));
  render(<InstructionFiles />);
  await open("^acme\\.md");
  fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "x" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("This file is read-only in Notted.");
  expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
});

test("a read-only file opens in Preview only, with the note and no toggle or Save", async () => {
  render(<InstructionFiles />);
  await open("base\\.md");
  expect(await screen.findByRole("heading", { name: "Base prompt", level: 1 })).toBeInTheDocument();
  expect(screen.getByText("This file is read-only in Notted.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Preview" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  expect(screen.queryByLabelText("File contents")).not.toBeInTheDocument();
});

test("a rejected read is shown verbatim", async () => {
  ReadInstructionFile.mockRejectedValue(new Error("That file is not in the archive."));
  render(<InstructionFiles />);
  await open("^acme\\.md");
  expect(await screen.findByRole("alert")).toHaveTextContent("That file is not in the archive.");
});

test("a rejected list is shown verbatim", async () => {
  ListInstructionFiles.mockRejectedValue(new Error("Set the archive folder to list its projects."));
  render(<InstructionFiles />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Set the archive folder to list its projects.");
});

test("choosing another file with unsaved changes asks first; Keep editing stays, Discard switches", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
  await open("^ana\\.md");
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByRole("heading", { name: "Discard unsaved changes?" })).toBeInTheDocument();
  expect(within(dialog).getByText("Your changes to instructions/projects/acme.md are not saved.")).toBeInTheDocument();
  fireEvent.click(within(dialog).getByRole("button", { name: "Keep editing" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByLabelText("File contents")).toHaveValue("edited");

  await open("^ana\\.md");
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Discard" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await screen.findByText("instructions/people/ana.md");
  expect(await screen.findByDisplayValue("# Ana")).toBeInTheDocument();
});

test("choosing another file with no unsaved changes switches without asking", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  await screen.findByLabelText("File contents");
  await open("^ana\\.md");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(await screen.findByDisplayValue("# Ana")).toBeInTheDocument();
});

test("the dialog keeps focus inside, handles Escape and Tab at document level, and removes its listener on close", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
  await open("^ana\\.md");
  const dialog = screen.getByRole("dialog");
  const keep = within(dialog).getByRole("button", { name: "Keep editing" });
  const discard = within(dialog).getByRole("button", { name: "Discard" });
  expect(dialog.contains(document.activeElement)).toBe(true);
  discard.focus();
  fireEvent.keyDown(document, { key: "Tab" });
  expect(document.activeElement).toBe(keep);
  fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
  expect(document.activeElement).toBe(discard);
  document.body.focus();
  fireEvent.keyDown(document, { key: "Tab" });
  expect(dialog.contains(document.activeElement)).toBe(true);
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByLabelText("File contents")).toHaveValue("edited");
  // after close, Tab is no longer trapped
  const before = document.activeElement;
  fireEvent.keyDown(document, { key: "Tab" });
  expect(document.activeElement).toBe(before);
});

test("a read that resolves after another file was chosen is ignored", async () => {
  let resolveSlow!: (t: string) => void;
  ReadInstructionFile.mockImplementation((p: string) =>
    p === "instructions/projects/acme.md" ? new Promise<string>((r) => (resolveSlow = r)) : Promise.resolve(TEXT[p] ?? ""),
  );
  render(<InstructionFiles />);
  await open("^acme\\.md");
  await open("^ana\\.md");
  expect(await screen.findByDisplayValue("# Ana")).toBeInTheDocument();
  await act(async () => resolveSlow("# LATE ACME"));
  expect(screen.getByLabelText("File contents")).toHaveValue("# Ana");
  expect(screen.getByText("instructions/people/ana.md")).toBeInTheDocument();
});

test("a save that resolves after another file was chosen does not mark the new file saved", async () => {
  let resolveWrite!: () => void;
  WriteInstructionFile.mockReturnValue(new Promise<void>((r) => (resolveWrite = r)));
  render(<InstructionFiles />);
  await open("^acme\\.md");
  fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await open("^ana\\.md");
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Discard" }));
  await screen.findByDisplayValue("# Ana");
  await act(async () => resolveWrite());
  expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
  expect(screen.getByLabelText("File contents")).toHaveValue("# Ana");
});

test("typing while a save is in flight keeps the newer text dirty", async () => {
  let resolveWrite!: () => void;
  WriteInstructionFile.mockReturnValue(new Promise<void>((r) => (resolveWrite = r)));
  render(<InstructionFiles />);
  await open("^acme\\.md");
  const box = await screen.findByLabelText("File contents");
  fireEvent.change(box, { target: { value: "first" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  fireEvent.change(box, { target: { value: "first and more" } });
  await act(async () => resolveWrite());
  expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
});

describe("open requests and archive changes", () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn();
  });

  test("a new openRequest reloads the list, opens the path and scrolls the card into view", async () => {
    const { rerender } = render(<InstructionFiles archivePath="/a" openRequest={null} />);
    await screen.findByText("Base prompt");
    ListInstructionFiles.mockClear();
    ListInstructionFiles.mockResolvedValue([...FILES, { path: "instructions/people/carla.md", kind: "person", readOnly: false }]);
    ReadInstructionFile.mockImplementation(async (p: string) => (p === "instructions/people/carla.md" ? "# Carla" : (TEXT[p] ?? "")));
    rerender(<InstructionFiles archivePath="/a" openRequest={{ path: "instructions/people/carla.md", seq: 1 }} />);
    expect(await screen.findByLabelText("File contents")).toHaveValue("# Carla");
    expect(ListInstructionFiles).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /^carla\.md/ })).toHaveAttribute("aria-current", "true");
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: "start" });
  });

  test("with an unsaved file, an openRequest asks before discarding", async () => {
    const { rerender } = render(<InstructionFiles archivePath="/a" openRequest={null} />);
    await open("^acme\\.md");
    fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
    rerender(<InstructionFiles archivePath="/a" openRequest={{ path: "instructions/people/ana.md", seq: 1 }} />);
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "Discard unsaved changes?" })).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Discard" }));
    expect(await screen.findByLabelText("File contents")).toHaveValue("# Ana");
  });

  test("an openRequest result that arrives after unmount does nothing", async () => {
    let resolve!: (v: unknown) => void;
    const { rerender, unmount } = render(<InstructionFiles archivePath="/a" openRequest={null} />);
    await screen.findByText("Base prompt");
    ListInstructionFiles.mockReturnValue(new Promise((r) => (resolve = r)));
    rerender(<InstructionFiles archivePath="/a" openRequest={{ path: "instructions/people/ana.md", seq: 1 }} />);
    unmount();
    ReadInstructionFile.mockClear();
    await act(async () => resolve(FILES));
    expect(ReadInstructionFile).not.toHaveBeenCalled();
  });

  test("a changed archivePath reloads the list", async () => {
    const { rerender } = render(<InstructionFiles archivePath="/a" openRequest={null} />);
    await screen.findByText("Base prompt");
    ListInstructionFiles.mockClear();
    rerender(<InstructionFiles archivePath="/b" openRequest={null} />);
    await act(async () => {});
    expect(ListInstructionFiles).toHaveBeenCalledTimes(1);
  });
});

describe("refresh", () => {
  test("a changed refresh reloads the list", async () => {
    const { rerender } = render(<InstructionFiles refresh={0} />);
    await screen.findByText("Base prompt");
    const before = ListInstructionFiles.mock.calls.length;
    rerender(<InstructionFiles refresh={1} />);
    await act(async () => {});
    expect(ListInstructionFiles.mock.calls.length).toBe(before + 1);
  });

  test("an open dirty file missing from the new list closes the editor and clears the dirty path", async () => {
    const onDirtyPath = vi.fn();
    const { rerender } = render(<InstructionFiles refresh={0} onDirtyPath={onDirtyPath} />);
    await open("^acme\\.md");
    fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
    expect(onDirtyPath).toHaveBeenLastCalledWith("instructions/projects/acme.md");
    ListInstructionFiles.mockResolvedValue(FILES.filter((f) => f.path !== "instructions/projects/acme.md"));
    rerender(<InstructionFiles refresh={1} onDirtyPath={onDirtyPath} />);
    expect(await screen.findByText("Choose a file to read or edit it.")).toBeInTheDocument();
    expect(onDirtyPath).toHaveBeenLastCalledWith(null);
  });

  test("a file still listed stays open, even when dirty", async () => {
    const { rerender } = render(<InstructionFiles refresh={0} />);
    await open("^acme\\.md");
    fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
    rerender(<InstructionFiles refresh={1} />);
    await act(async () => {});
    expect(screen.getByLabelText("File contents")).toHaveValue("edited");
  });
});
