import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { InstructionFiles } from "./InstructionFiles";
import { ListInstructionFiles, ReadInstructionFile, WriteInstructionFile, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));
vi.mock("@uiw/react-codemirror", () => import("../test/codemirrorMock"));

const FILES = [
  { path: "prompts/base.md", kind: "base", readOnly: true },
  { path: "instructions/README.md", kind: "readme", readOnly: false },
  { path: "instructions/clients/acme.md", kind: "client", readOnly: false },
  { path: "instructions/people/ana.md", kind: "person", readOnly: false },
  { path: "instructions/people/ana/acme.md", kind: "person-client", readOnly: false },
  { path: "instructions/local/notes.md", kind: "local", readOnly: false },
];
const TEXT: Record<string, string> = {
  "prompts/base.md": "# Base prompt\n\nAlways be brief.",
  "instructions/README.md": "# Guide",
  "instructions/clients/acme.md": "# Acme\n\nUse the acme glossary.",
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
  expect(labels).toEqual(["Base prompt", "Guide", "Clients", "People", "Per client", "Local overrides"]);
  expect(screen.getByText("Instruction files")).toBeInTheDocument();
  expect(screen.getByText("Choose a file to read or edit it.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /ana\/acme\.md/ })).toBeInTheDocument();
  expect(within(screen.getByRole("button", { name: /base\.md/ })).getByText("Read-only")).toBeInTheDocument();
  expect(within(screen.getByRole("button", { name: /^acme\.md/ })).queryByText("Read-only")).not.toBeInTheDocument();
});

test("opening a file reads it and shows the editor with Edit and Preview", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  expect(ReadInstructionFile).toHaveBeenCalledWith("instructions/clients/acme.md");
  expect(await screen.findByLabelText("File contents")).toHaveValue("# Acme\n\nUse the acme glossary.");
  expect(screen.getByText("instructions/clients/acme.md")).toBeInTheDocument();
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
  expect(WriteInstructionFile).toHaveBeenCalledWith("instructions/clients/acme.md", "# Acme\n\nNew rule.");
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
  expect(WriteInstructionFile).toHaveBeenCalledWith("instructions/clients/acme.md", "changed");
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
  ListInstructionFiles.mockRejectedValue(new Error("Set the archive folder to list its clients."));
  render(<InstructionFiles />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Set the archive folder to list its clients.");
});

test("choosing another file with unsaved changes asks first; Keep editing stays, Discard switches", async () => {
  render(<InstructionFiles />);
  await open("^acme\\.md");
  fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
  await open("^ana\\.md");
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByRole("heading", { name: "Discard unsaved changes?" })).toBeInTheDocument();
  expect(within(dialog).getByText("Your changes to instructions/clients/acme.md are not saved.")).toBeInTheDocument();
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
    p === "instructions/clients/acme.md" ? new Promise<string>((r) => (resolveSlow = r)) : Promise.resolve(TEXT[p] ?? ""),
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
