import { fireEvent, render, screen, within } from "@testing-library/react";
import { App } from "./App";
import { GetMeeting, GetSettings, ListInstructionFiles, ListMeetings, ReadInstructionFile, meeting, resetBindings } from "./test/bindingsMock";

vi.mock("./api/bindings", () => import("./test/bindingsMock"));
vi.mock("@uiw/react-codemirror", () => import("./test/codemirrorMock"));

beforeEach(resetBindings);

test("shows the title bar, the sidebar and the new-recording view when there are no meetings", async () => {
  render(<App />);
  expect(screen.getByText("Notted")).toBeInTheDocument();
  expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
});

test("opens the newest meeting on launch", async () => {
  ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" }), meeting({ id: "m1", title: "Older" })]);
  GetMeeting.mockImplementation(async (id: string) => meeting({ id, title: id === "m2" ? "Newest" : "Older" }));
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
});

test("an empty saved archive opens Settings with the Set up Notted card, even when meetings exist", async () => {
  ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" })]);
  GetSettings.mockResolvedValue({ profile: "", checkoutPath: "", projectRoots: {} });
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
  expect(await screen.findByText("Set up Notted")).toBeInTheDocument();
});

test("a GetSettings rejection falls back to the newest meeting", async () => {
  ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" })]);
  GetMeeting.mockResolvedValue(meeting({ id: "m2", title: "Newest" }));
  GetSettings.mockRejectedValue(new Error("no settings"));
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
});

describe("leaving Settings with an unsaved instruction file", () => {
  async function openDirtyFile() {
    ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" })]);
    GetMeeting.mockResolvedValue(meeting({ id: "m2", title: "Newest" }));
    ListInstructionFiles.mockResolvedValue([{ path: "instructions/projects/acme.md", kind: "project", readOnly: false }]);
    ReadInstructionFile.mockResolvedValue("# Acme");
    render(<App initialView={{ kind: "settings" }} />);
    fireEvent.click(await screen.findByRole("button", { name: /^acme\.md/ }));
    fireEvent.change(await screen.findByLabelText("File contents"), { target: { value: "edited" } });
    await screen.findByText("Unsaved changes");
  }

  test("choosing a meeting asks first; Keep editing stays, Discard navigates", async () => {
    await openDirtyFile();
    fireEvent.click(await screen.findByRole("button", { name: /Newest/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "Discard unsaved changes?" })).toBeInTheDocument();
    expect(within(dialog).getByText("Your changes to instructions/projects/acme.md are not saved.")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Keep editing" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByLabelText("File contents")).toHaveValue("edited");
    fireEvent.click(screen.getByRole("button", { name: /Newest/ }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Discard" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  test("New recording asks first and Escape keeps editing", async () => {
    await openDirtyFile();
    fireEvent.click(screen.getByRole("button", { name: "New recording" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByLabelText("File contents")).toBeInTheDocument();
  });

  test("a clean file leaves Settings without asking", async () => {
    ListInstructionFiles.mockResolvedValue([]);
    render(<App initialView={{ kind: "settings" }} />);
    await screen.findByLabelText("Profile");
    fireEvent.click(screen.getByRole("button", { name: "New recording" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  });
});
