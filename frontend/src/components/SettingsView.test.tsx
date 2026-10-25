import { fireEvent, render, screen } from "@testing-library/react";
import { SettingsView } from "./SettingsView";
import { GetPermissions, GetUpdateStatus, GetSettings, ListMyProjectKeys, ListProfiles, ListProjects, ListInstructionFiles, DeleteProfile, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));
vi.mock("@uiw/react-codemirror", () => import("../test/codemirrorMock"));

beforeEach(() => {
  resetBindings();
  ListInstructionFiles.mockResolvedValue([{ path: "instructions/projects/acme.md", kind: "project", readOnly: false }]);
});

test("shows the Settings heading, one selected Instructions tab and the cards in order", async () => {
  render(<SettingsView />);
  expect(screen.getByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
  const tabs = screen.getAllByRole("tab");
  expect(tabs).toHaveLength(1);
  expect(tabs[0]).toHaveTextContent("Instructions");
  expect(tabs[0]).toHaveAttribute("aria-selected", "true");
  await screen.findByLabelText("Profile");
  const headers = [...document.querySelectorAll("section > .nt-card__header")].map((e) => e.textContent);
  expect(headers).toEqual(["Set up Notted", "Your setup", "Instruction files", "Prompt preview", "Update Notted", "Permissions"]);
  expect(GetUpdateStatus).toHaveBeenCalledTimes(1);
  expect(GetPermissions).toHaveBeenCalledTimes(1);
});

test("the project list feeds the project-root rows and the preview select; a failure shows the message in both", async () => {
  render(<SettingsView />);
  expect(await screen.findByLabelText("acme")).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "globex" })).toBeInTheDocument();
});

test("a refused project list shows its message", async () => {
  ListProjects.mockRejectedValue(new Error("Set the archive folder to list its projects."));
  render(<SettingsView />);
  expect(await screen.findAllByText("Set the archive folder to list its projects.")).not.toHaveLength(0);
});

test("there is no model, provider or API key control or text", async () => {
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "/archive", projectRoots: {} });
  render(<SettingsView />);
  await screen.findByLabelText("Profile");
  const controls = [...screen.queryAllByRole("combobox"), ...screen.queryAllByRole("textbox"), ...screen.queryAllByRole("button"), ...screen.queryAllByRole("tab")];
  expect(controls.length).toBeGreaterThan(3);
  for (const c of controls) {
    const label = (c as HTMLInputElement).labels?.[0]?.textContent ?? "";
    const name = `${c.getAttribute("aria-label") ?? ""} ${c.textContent ?? ""} ${label} ${c.getAttribute("placeholder") ?? ""}`;
    expect(name).not.toMatch(/model|provider|api key/i);
  }
  expect(document.body.textContent).not.toMatch(/model|provider|api key/i);
});

test("the welcome card shows while the profile is unset and hides when archive and profile are set", async () => {
  GetSettings.mockResolvedValue({ profile: "", checkoutPath: "/archive", projectRoots: {} });
  const { unmount } = render(<SettingsView />);
  expect(await screen.findByText("Set up Notted")).toBeInTheDocument();
  unmount();
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "/archive", projectRoots: {} });
  render(<SettingsView />);
  await screen.findByLabelText("Profile");
  expect(screen.queryByText("Set up Notted")).not.toBeInTheDocument();
});

test("the welcome card shows while the archive is unset, and no list is requested", async () => {
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "", projectRoots: {} });
  render(<SettingsView />);
  expect(await screen.findByText("Set up Notted")).toBeInTheDocument();
  await screen.findByLabelText("Profile");
  expect(ListProjects).not.toHaveBeenCalled();
  expect(ListProfiles).not.toHaveBeenCalled();
  expect(screen.getAllByText("Choose the archive folder first.")).not.toHaveLength(0);
});

test("a set archive loads the projects and the profiles", async () => {
  render(<SettingsView />);
  await screen.findByLabelText("acme");
  expect(ListProjects).toHaveBeenCalled();
  expect(ListProfiles).toHaveBeenCalled();
  expect(screen.getByRole("option", { name: "bruno" })).toBeInTheDocument();
});

test("ListMyProjectKeys is not called without an archive or without a profile", async () => {
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "", projectRoots: {} });
  const { unmount } = render(<SettingsView />);
  await screen.findByLabelText("Profile");
  unmount();
  GetSettings.mockResolvedValue({ profile: "", checkoutPath: "/archive", projectRoots: {} });
  render(<SettingsView />);
  await screen.findByLabelText("acme");
  expect(ListMyProjectKeys).not.toHaveBeenCalled();
});

test("my project keys reach the form: a key it lacks gets Create your file", async () => {
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "/archive", projectRoots: {} });
  ListMyProjectKeys.mockResolvedValue(["acme"]);
  render(<SettingsView />);
  expect(await screen.findByRole("button", { name: "Create your file for globex" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Create your file for acme" })).not.toBeInTheDocument();
});

test("a refused key list shows its message", async () => {
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "/archive", projectRoots: {} });
  ListMyProjectKeys.mockRejectedValue(new Error("Could not list your project files."));
  render(<SettingsView />);
  expect(await screen.findByText("Could not list your project files.")).toBeInTheDocument();
});

test("after a delete, settings, profiles, projects, my keys and the file list reload", async () => {
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "/archive", projectRoots: {} });
  render(<SettingsView />);
  fireEvent.click(await screen.findByRole("button", { name: "Delete a profile…" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete profile bruno" }));
  await screen.findByRole("dialog");
  const calls = [GetSettings, ListProfiles, ListProjects, ListMyProjectKeys, ListInstructionFiles].map((f) => f.mock.calls.length);
  fireEvent.click(screen.getByRole("button", { name: "Delete profile" }));
  await screen.findByText("Deleted profile bruno from the archive folder.");
  expect(DeleteProfile).toHaveBeenCalled();
  const after = [GetSettings, ListProfiles, ListProjects, ListMyProjectKeys, ListInstructionFiles].map((f) => f.mock.calls.length);
  after.forEach((n, i) => expect(n).toBeGreaterThan(calls[i]));
});
