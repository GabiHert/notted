import { act, fireEvent, render, screen } from "@testing-library/react";
import { SetupForm } from "./SetupForm";
import { GetSettings, SetSettings, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(() => {
  resetBindings();
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } });
});

const props = { clients: ["acme", "globex"], clientsError: "", onSaved: () => {} };

test("fills the fields from the saved settings and shows a row per client", async () => {
  render(<SetupForm {...props} />);
  expect(await screen.findByLabelText("Profile")).toHaveValue("ana");
  expect(screen.getByLabelText("Archive folder")).toHaveValue("/archive");
  expect(screen.getByLabelText("acme")).toHaveValue("/work/acme");
  expect(screen.getByLabelText("globex")).toHaveValue("");
  expect(screen.getByLabelText("globex")).toHaveAttribute("placeholder", "/full/path/to/the/project");
  expect(screen.getByText("Your setup")).toBeInTheDocument();
  expect(screen.getByText("Your file in instructions/people/, without .md (for example ana).")).toBeInTheDocument();
  expect(screen.getByText("The full path of your local clone of the meetings archive.")).toBeInTheDocument();
  expect(screen.getByText("Project roots")).toBeInTheDocument();
  expect(screen.getByText("Optional. A folder per client that Claude Code may read for that client's meetings.")).toBeInTheDocument();
});

test("Save settings sends the typed values and the roots map, then says Settings saved.", async () => {
  const onSaved = vi.fn();
  render(<SetupForm {...props} onSaved={onSaved} />);
  const profile = await screen.findByLabelText("Profile");
  fireEvent.change(profile, { target: { value: "bruno" } });
  fireEvent.change(screen.getByLabelText("globex"), { target: { value: "/work/globex" } });
  fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
  expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
  expect(SetSettings).toHaveBeenCalledWith({ profile: "bruno", checkoutPath: "/archive", projectRoots: { acme: "/work/acme", globex: "/work/globex" } });
  expect(onSaved).toHaveBeenCalledTimes(1);
});

test("Settings saved. fades after 3 seconds", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  try {
    render(<SetupForm {...props} />);
    await screen.findByLabelText("Profile");
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    expect(await screen.findByText("Settings saved.")).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(3100);
    });
    expect(screen.queryByText("Settings saved.")).not.toBeInTheDocument();
  } finally {
    vi.useRealTimers();
  }
});

test("a rejection is shown verbatim in an error card and no saved message appears", async () => {
  SetSettings.mockRejectedValue(new Error("The archive folder does not exist: /nope"));
  render(<SetupForm {...props} />);
  await screen.findByLabelText("Profile");
  fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("The archive folder does not exist: /nope");
  expect(screen.queryByText("Settings saved.")).not.toBeInTheDocument();
});

test("when the client list is refused, the message is shown and the saved roots are kept", async () => {
  render(<SetupForm clients={[]} clientsError="Set the archive folder to list its clients." onSaved={() => {}} />);
  await screen.findByLabelText("Profile");
  expect(screen.getByText("Set the archive folder to list its clients.")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
  await screen.findByText("Settings saved.");
  expect(SetSettings).toHaveBeenCalledWith({ profile: "ana", checkoutPath: "/archive", projectRoots: { acme: "/work/acme" } });
});

test("a save that resolves after unmount does not touch state or call onSaved", async () => {
  let resolve!: () => void;
  SetSettings.mockReturnValue(new Promise<void>((r) => (resolve = r)));
  const onSaved = vi.fn();
  const { unmount } = render(<SetupForm {...props} onSaved={onSaved} />);
  await screen.findByLabelText("Profile");
  fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
  unmount();
  await act(async () => resolve());
  expect(onSaved).not.toHaveBeenCalled();
});
