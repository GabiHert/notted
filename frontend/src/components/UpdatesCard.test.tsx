import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { UpdatesCard, type UpdateStatus, type UpdatesAPI } from "./UpdatesCard";

const base: UpdateStatus = {
  version: "v0.1.0",
  repo: "GabiHert/notted",
  latest: null,
  upToDate: false,
  hasToken: false,
  state: "idle",
  message: "",
  lines: [],
  seq: 0,
  installedApp: "/Applications/Phonon.app",
};
const release = { tag: "v0.2.0", name: "Phonon v0.2.0", notes: "", date: 0, url: "u" };

type Handler = (data: any) => void;
function setup(status: UpdateStatus, over: Partial<UpdatesAPI> = {}) {
  const handlers = new Map<string, Handler>();
  const api: UpdatesAPI = {
    getStatus: vi.fn().mockResolvedValue(status),
    check: vi.fn().mockResolvedValue(status),
    start: vi.fn().mockResolvedValue(undefined),
    setToken: vi.fn().mockResolvedValue(status),
    ...over,
  };
  const on = (event: string, cb: Handler) => {
    handlers.set(event, cb);
    return () => handlers.delete(event);
  };
  const r = render(<UpdatesCard appName="Phonon" api={api} on={on} />);
  return { api, handlers, ...r };
}

test("shows the running version and the releases it updates from, with no source folder", async () => {
  setup(base);
  expect(await screen.findByText("v0.1.0")).toBeInTheDocument();
  expect(screen.getByText("GabiHert/notted")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Check for updates" })).toBeEnabled();
  expect(screen.queryByText(/source folder/i)).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Update Phonon" })).not.toBeInTheDocument();
});

test("Check for updates shows a newer release and offers Update Phonon, which starts the update", async () => {
  const { api } = setup(base, { check: vi.fn().mockResolvedValue({ ...base, latest: release }) });
  fireEvent.click(await screen.findByRole("button", { name: "Check for updates" }));
  expect(await screen.findByText("v0.2.0")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Update Phonon" }));
  await waitFor(() => expect(api.start).toHaveBeenCalledTimes(1));
});

test("says Phonon is up to date when the newest release is the running one", async () => {
  setup({ ...base, latest: { ...release, tag: "v0.1.0" }, upToDate: true });
  expect(await screen.findByText("Phonon is up to date.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Update Phonon" })).not.toBeInTheDocument();
});

test("a start error, such as a running recording, shows verbatim", async () => {
  const msg = "Stop the recording first. Phonon quits and reopens to finish the update.";
  setup({ ...base, latest: release }, { start: vi.fn().mockRejectedValue(new Error(msg)) });
  fireEvent.click(await screen.findByRole("button", { name: "Update Phonon" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(msg);
});

test("states and output lines arrive as events; a failure shows its message and the output", async () => {
  const { handlers, api } = setup({ ...base, latest: release });
  await screen.findByText("v0.2.0");
  act(() => handlers.get("update:state")!({ state: "downloading", message: "" }));
  expect(screen.getByRole("status")).toHaveTextContent("Downloading…");
  expect(screen.getByRole("button", { name: "Check for updates" })).toBeDisabled();
  act(() => handlers.get("update:output")!({ seq: 1, line: "Downloading Phonon-v0.2.0-darwin-arm64.zip" }));
  expect(screen.getByTestId("update-output")).toHaveTextContent("Downloading Phonon-v0.2.0-darwin-arm64.zip");
  (api.getStatus as ReturnType<typeof vi.fn>).mockResolvedValue({ ...base, latest: release, state: "failed", message: "Checksum mismatch. Phonon did not change.", lines: ["x"], seq: 1 });
  act(() => handlers.get("update:state")!({ state: "failed", message: "Checksum mismatch. Phonon did not change." }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Checksum mismatch. Phonon did not change.");
});

test("a copy macOS runs from App Translocation explains how to move it and cannot update", async () => {
  setup({ ...base, latest: release, installedApp: "/private/var/folders/x/AppTranslocation/1/d/Phonon.app" });
  expect(await screen.findByRole("alert")).toHaveTextContent("drag Phonon.app into Applications in Finder");
  expect(screen.getByRole("button", { name: "Update Phonon" })).toBeDisabled();
});

test("a token is saved in the keychain from the token form", async () => {
  const { api } = setup(base, { setToken: vi.fn().mockResolvedValue({ ...base, hasToken: true }) });
  fireEvent.click(await screen.findByRole("button", { name: "Add token" }));
  fireEvent.change(screen.getByLabelText("GitHub token"), { target: { value: "github_pat_abc" } });
  fireEvent.click(screen.getByRole("button", { name: "Save in keychain" }));
  await waitFor(() => expect(api.setToken).toHaveBeenCalledWith("github_pat_abc"));
  expect(await screen.findByText(/A GitHub token is saved in your keychain/)).toBeInTheDocument();
});
