import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { PermissionsCard } from "./PermissionsCard";
import { AskPermissions, GetPermissions, OpenPermissionSettings, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

const HINT = "If it is already on in System Settings, select Notted there, remove it with the minus button, then click Check again.";
const perms = (microphone: string, screenState: string) => ({ microphone, screen: screenState });
const row = (name: string) => screen.getByRole("listitem", { name });
const MIC = "Microphone";
const SCREEN = "Screen & System Audio Recording";

async function loaded(microphone = "allowed", screenState = "allowed") {
  GetPermissions.mockResolvedValue(perms(microphone, screenState));
  const r = render(<PermissionsCard />);
  await screen.findAllByText(/^(Allowed|Not allowed|Not asked yet)$/);
  return r;
}

beforeEach(() => {
  resetBindings();
});

test("renders the header, both rows and Check again", async () => {
  await loaded();
  expect(screen.getByText("Permissions", { selector: ".nt-card__header" })).toBeInTheDocument();
  expect(within(row(MIC)).getByText("Allowed")).toBeInTheDocument();
  expect(within(row(SCREEN)).getByText("Allowed")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Check again" })).toBeEnabled();
});

test("checks once when it opens, and never asks", async () => {
  await loaded();
  expect(GetPermissions).toHaveBeenCalledTimes(1);
  expect(AskPermissions).not.toHaveBeenCalled();
});

test.each([
  ["allowed", "Allowed"],
  ["denied", "Not allowed"],
  ["not_asked", "Not asked yet"],
])("shows %s as %s for the microphone", async (state, text) => {
  await loaded(state, "allowed");
  expect(within(row(MIC)).getByText(text)).toBeInTheDocument();
});

test.each([
  ["allowed", "Allowed"],
  ["denied", "Not allowed"],
])("shows %s as %s for screen recording", async (state, text) => {
  await loaded("allowed", state);
  expect(within(row(SCREEN)).getByText(text)).toBeInTheDocument();
});

test("each row has Open System Settings, and Ask appears only while the status is Not asked yet", async () => {
  await loaded("not_asked", "denied");
  expect(within(row(MIC)).getByRole("button", { name: "Open System Settings" })).toBeInTheDocument();
  expect(within(row(MIC)).getByRole("button", { name: "Ask" })).toBeInTheDocument();
  expect(within(row(SCREEN)).getByRole("button", { name: "Open System Settings" })).toBeInTheDocument();
  expect(within(row(SCREEN)).queryByRole("button", { name: "Ask" })).not.toBeInTheDocument();
});

test("Ask is hidden for Allowed and Not allowed", async () => {
  await loaded("denied", "allowed");
  expect(screen.queryByRole("button", { name: "Ask" })).not.toBeInTheDocument();
});

test("the stale-permission hint shows under a Not allowed row only", async () => {
  await loaded("denied", "allowed");
  expect(within(row(MIC)).getByText(HINT)).toBeInTheDocument();
  expect(within(row(SCREEN)).queryByText(HINT)).not.toBeInTheDocument();
});

test("no hint when nothing is Not allowed", async () => {
  await loaded("not_asked", "allowed");
  expect(screen.queryByText(HINT)).not.toBeInTheDocument();
});

test("Open System Settings opens the pane of its own row", async () => {
  OpenPermissionSettings.mockResolvedValue(undefined);
  await loaded("denied", "denied");
  fireEvent.click(within(row(SCREEN)).getByRole("button", { name: "Open System Settings" }));
  expect(OpenPermissionSettings).toHaveBeenCalledWith("screen");
  fireEvent.click(within(row(MIC)).getByRole("button", { name: "Open System Settings" }));
  expect(OpenPermissionSettings).toHaveBeenLastCalledWith("microphone");
});

test("a failure to open System Settings shows its message", async () => {
  OpenPermissionSettings.mockRejectedValue(new Error("Could not open System Settings."));
  await loaded("denied", "allowed");
  fireEvent.click(within(row(MIC)).getByRole("button", { name: "Open System Settings" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Could not open System Settings.");
});

test("Ask shows the answer macOS gave", async () => {
  AskPermissions.mockResolvedValue(perms("allowed", "allowed"));
  await loaded("not_asked", "allowed");
  fireEvent.click(screen.getByRole("button", { name: "Ask" }));
  await waitFor(() => expect(within(row(MIC)).getByText("Allowed")).toBeInTheDocument());
  expect(AskPermissions).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole("button", { name: "Ask" })).not.toBeInTheDocument();
});

test("Check again reads the permissions again, without asking", async () => {
  await loaded("denied", "allowed");
  GetPermissions.mockResolvedValue(perms("allowed", "allowed"));
  fireEvent.click(screen.getByRole("button", { name: "Check again" }));
  await waitFor(() => expect(within(row(MIC)).getByText("Allowed")).toBeInTheDocument());
  expect(GetPermissions).toHaveBeenCalledTimes(2);
  expect(AskPermissions).not.toHaveBeenCalled();
  expect(screen.queryByText(HINT)).not.toBeInTheDocument();
});

test("checks again when the window regains focus, and stops listening on unmount", async () => {
  const { unmount } = await loaded("denied", "allowed");
  GetPermissions.mockResolvedValue(perms("allowed", "allowed"));
  act(() => {
    window.dispatchEvent(new Event("focus"));
  });
  await waitFor(() => expect(within(row(MIC)).getByText("Allowed")).toBeInTheDocument());
  expect(GetPermissions).toHaveBeenCalledTimes(2);
  unmount();
  window.dispatchEvent(new Event("focus"));
  expect(GetPermissions).toHaveBeenCalledTimes(2);
});

test("a helper failure shows the message and keeps Check again usable", async () => {
  GetPermissions.mockRejectedValue(new Error("Notted could not read the permissions: the audio recorder did not answer."));
  render(<PermissionsCard />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Notted could not read the permissions");
  expect(screen.queryByText("Allowed")).not.toBeInTheDocument();
  GetPermissions.mockResolvedValue(perms("allowed", "allowed"));
  fireEvent.click(screen.getByRole("button", { name: "Check again" }));
  await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
  expect(within(row(MIC)).getByText("Allowed")).toBeInTheDocument();
});

test("a late answer to an older check never overwrites a newer one", async () => {
  let release: (v: unknown) => void = () => {};
  GetPermissions.mockReturnValueOnce(new Promise((r) => (release = r)));
  render(<PermissionsCard />);
  GetPermissions.mockResolvedValue(perms("allowed", "allowed"));
  act(() => {
    window.dispatchEvent(new Event("focus"));
  });
  await waitFor(() => expect(within(row(MIC)).getByText("Allowed")).toBeInTheDocument());
  await act(async () => release(perms("denied", "denied")));
  expect(within(row(MIC)).getByText("Allowed")).toBeInTheDocument();
});

test("buttons are disabled while a check or ask is running", async () => {
  await loaded("not_asked", "allowed");
  let release: (v: unknown) => void = () => {};
  AskPermissions.mockReturnValue(new Promise((r) => (release = r)));
  fireEvent.click(screen.getByRole("button", { name: "Ask" }));
  expect(screen.getByRole("button", { name: "Check again" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Ask" })).toBeDisabled();
  await act(async () => release(perms("allowed", "allowed")));
  expect(screen.getByRole("button", { name: "Check again" })).toBeEnabled();
});
