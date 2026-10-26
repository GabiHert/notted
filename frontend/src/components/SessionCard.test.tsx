import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SessionCard } from "./SessionCard";
import { EventsOff, GetSettings, ResizeSession, StartSession, StopSession, WriteSession, emit, listenerCount, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

const fake = vi.hoisted(() => {
  const state = { terms: [] as any[] };
  class Terminal {
    written: string[] = [];
    disposed = false;
    cols = 80;
    rows = 24;
    options: Record<string, any>;
    dataCb: (d: string) => void = () => {};
    resizeCb: (s: { cols: number; rows: number }) => void = () => {};
    constructor(options: Record<string, any>) {
      this.options = options;
      state.terms.push(this);
    }
    open() {}
    keyHandler: (e: any) => boolean = () => true;
    attachCustomKeyEventHandler(cb: (e: any) => boolean) {
      this.keyHandler = cb;
    }
    loadAddon() {}
    focus() {}
    write(d: string) {
      this.written.push(d);
    }
    onData(cb: (d: string) => void) {
      this.dataCb = cb;
      return { dispose() {} };
    }
    onResize(cb: (s: { cols: number; rows: number }) => void) {
      this.resizeCb = cb;
      return { dispose() {} };
    }
    dispose() {
      this.disposed = true;
    }
  }
  return { state, Terminal };
});
vi.mock("@xterm/xterm", () => ({ Terminal: fake.Terminal }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));

const lastTerm = () => fake.state.terms[fake.state.terms.length - 1];

beforeEach(() => {
  resetBindings();
  fake.state.terms.length = 0;
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} unobserve() {} });
});

const NO_SESSION = "This meeting has no Claude Code session yet. Generate the summary first.";
const NO_FOLDER = "Set the meetings archive folder in Settings > Instructions first.";

async function opened() {
  render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(StartSession).toHaveBeenCalled());
  await waitFor(() => expect(fake.state.terms.length).toBe(1));
}

test("no session: disabled with the exact reason and no StartSession", async () => {
  render(<SessionCard meetingId="m1" hasSession={false} />);
  expect(await screen.findByText(NO_SESSION)).toBeInTheDocument();
  const btn = screen.getByRole("button", { name: "Open session" });
  expect(btn).toBeDisabled();
  fireEvent.click(btn);
  expect(StartSession).not.toHaveBeenCalled();
  expect(screen.getByText("Claude Code session")).toBeInTheDocument();
});

test("empty archive folder: disabled with the other reason", async () => {
  GetSettings.mockResolvedValue({ profile: "", checkoutPath: "", projectRoots: {} });
  render(<SessionCard meetingId="m1" hasSession />);
  expect(await screen.findByText(NO_FOLDER)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Open session" })).toBeDisabled();
  expect(screen.queryByText(NO_SESSION)).toBeNull();
});

test("closed state shows the invitation text", async () => {
  render(<SessionCard meetingId="m1" hasSession />);
  expect(await screen.findByText("Continue this meeting's Claude Code conversation here. Claude asks before it changes anything, and your answer is the yes.")).toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole("button", { name: "Open session" })).toBeEnabled());
});

test("Open session starts the session and shows the terminal with Close session", async () => {
  await opened();
  expect(StartSession).toHaveBeenCalledWith("m1");
  expect(screen.getByRole("button", { name: "Close session" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Open session" })).toBeNull();
});

test("a StartSession rejection shows the message verbatim", async () => {
  StartSession.mockRejectedValue(new Error("Claude Code is not installed."));
  render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  expect(await screen.findByText("Claude Code is not installed.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Open session" })).toBeEnabled();
});

test("data events for this meeting are written; other meetings are not", async () => {
  await opened();
  act(() => emit("terminal:data:m1", "hello"));
  act(() => emit("terminal:data:m2", "nope"));
  expect(lastTerm().written).toEqual(["hello"]);
});

test("keystrokes go to WriteSession and size changes to ResizeSession", async () => {
  await opened();
  act(() => lastTerm().dataCb("y\r"));
  expect(WriteSession).toHaveBeenCalledWith("m1", "y\r");
  act(() => lastTerm().resizeCb({ cols: 100, rows: 30 }));
  expect(ResizeSession).toHaveBeenCalledWith("m1", 100, 30);
});

test("the app never writes to the session on its own", async () => {
  await opened();
  act(() => emit("terminal:data:m1", "Do you want to proceed? (y/n)"));
  act(() => emit("terminal:data:m1", "1. Yes\n2. No"));
  expect(WriteSession).not.toHaveBeenCalled();
  act(() => lastTerm().dataCb("n"));
  expect(WriteSession).toHaveBeenCalledTimes(1);
  expect(WriteSession).toHaveBeenCalledWith("m1", "n");
});

test("exit shows the text and Restart session starts again", async () => {
  await opened();
  act(() => emit("terminal:exit:m1", { code: 0 }));
  expect(screen.getByText("The session ended (exit code 0).")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Restart session" }));
  await waitFor(() => expect(StartSession).toHaveBeenCalledTimes(2));
  expect(StartSession).toHaveBeenLastCalledWith("m1");
  expect(screen.queryByText(/The session ended/)).toBeNull();
  expect(screen.getByRole("button", { name: "Close session" })).toBeInTheDocument();
});

test("exit event for another meeting is ignored", async () => {
  await opened();
  act(() => emit("terminal:exit:m2", { code: 1 }));
  expect(screen.queryByText(/The session ended/)).toBeNull();
});

test("Close session calls StopSession and returns to the closed card; a code -1 exit is a user stop", async () => {
  await opened();
  fireEvent.click(screen.getByRole("button", { name: "Close session" }));
  expect(StopSession).toHaveBeenCalledWith("m1");
  act(() => emit("terminal:exit:m1", { code: -1 }));
  expect(screen.queryByText(/The session ended/)).toBeNull();
  expect(screen.getByRole("button", { name: "Open session" })).toBeInTheDocument();
});

test("leaving the page stops the session and drops the listeners", async () => {
  const { unmount } = render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(fake.state.terms.length).toBe(1));
  expect(listenerCount("terminal:data:m1")).toBe(1);
  unmount();
  expect(StopSession).toHaveBeenCalledWith("m1");
  expect(lastTerm().disposed).toBe(true);
  expect(listenerCount("terminal:data:m1")).toBe(0);
  expect(listenerCount("terminal:exit:m1")).toBe(0);
  expect(EventsOff).toHaveBeenCalledWith("terminal:data:m1");
  expect(EventsOff).toHaveBeenCalledWith("terminal:exit:m1");
});

test("unmount before a session was opened does not stop anything", async () => {
  const { unmount } = render(<SessionCard meetingId="m1" hasSession />);
  await screen.findByRole("button", { name: "Open session" });
  unmount();
  expect(StopSession).not.toHaveBeenCalled();
});

test("terminal theme comes from the Ledger CSS variables", async () => {
  document.documentElement.style.setProperty("--card", "#ffffff");
  document.documentElement.style.setProperty("--fg", "#1d2330");
  document.documentElement.style.setProperty("--accent", "#24408e");
  document.documentElement.style.setProperty("--sel", "#e3e7f5");
  await opened();
  expect(lastTerm().options.theme).toMatchObject({ background: "#ffffff", foreground: "#1d2330", cursor: "#24408e", selectionBackground: "#e3e7f5" });
  expect(lastTerm().options).toMatchObject({ fontSize: 12.5, cursorBlink: true, scrollback: 5000 });
});

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((r) => (resolve = r));
  return { promise, resolve };
}

test("unmount while StartSession is pending stops the session once it resolves", async () => {
  const d = deferred();
  StartSession.mockReturnValue(d.promise);
  const { unmount } = render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(StartSession).toHaveBeenCalled());
  unmount();
  StopSession.mockClear();
  await act(async () => d.resolve());
  expect(StopSession).toHaveBeenCalledWith("m1");
});

test("Close session while StartSession is pending stops the session once it resolves", async () => {
  const d = deferred();
  StartSession.mockReturnValue(d.promise);
  render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(StartSession).toHaveBeenCalled());
  fireEvent.click(screen.getByRole("button", { name: "Close session" }));
  StopSession.mockClear();
  await act(async () => d.resolve());
  expect(StopSession).toHaveBeenCalledWith("m1");
});

test("a session that starts normally is not stopped when StartSession resolves", async () => {
  await opened();
  await act(async () => {});
  expect(StopSession).not.toHaveBeenCalled();
});

function keyEvent(shiftKey: boolean, type = "keydown") {
  return { type, key: "Tab", ctrlKey: true, shiftKey, altKey: false, metaKey: false };
}

test("the terminal shows the leave hint and points aria-describedby at it", async () => {
  await opened();
  const hint = screen.getByText("Press Ctrl+Tab to leave the terminal.");
  expect(screen.getByTestId("terminal")).toHaveAttribute("aria-describedby", hint.id);
  expect(hint.id).not.toBe("");
});

test("Ctrl+Tab and Ctrl+Shift+Tab move focus out of the terminal and never reach WriteSession", async () => {
  render(
    <>
      <button>before</button>
      <SessionCard meetingId="m1" hasSession />
      <button>after</button>
    </>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(fake.state.terms.length).toBe(1));
  const term = lastTerm();
  expect(term.keyHandler(keyEvent(false))).toBe(false);
  expect(screen.getByRole("button", { name: "after" })).toHaveFocus();
  expect(term.keyHandler(keyEvent(true))).toBe(false);
  expect(screen.getByRole("button", { name: "Close session" })).toHaveFocus();
  expect(term.keyHandler(keyEvent(false, "keyup"))).toBe(false);
  expect(WriteSession).not.toHaveBeenCalled();
});

test("Escape, plain Tab and Shift+Tab stay with the session", async () => {
  await opened();
  const term = lastTerm();
  expect(term.keyHandler({ type: "keydown", key: "Escape", ctrlKey: false, shiftKey: false })).toBe(true);
  expect(term.keyHandler({ type: "keydown", key: "Tab", ctrlKey: false, shiftKey: false })).toBe(true);
  expect(term.keyHandler({ type: "keydown", key: "Tab", ctrlKey: false, shiftKey: true })).toBe(true);
});

// The backend keys sessions by meeting, and its Stop emits terminal:exit with code -1 before it returns.
function stopEmitsUserExit() {
  StopSession.mockImplementation(async (id: string) => {
    emit("terminal:exit:" + id, { code: -1 });
  });
}

const order = (fn: { mock: { invocationCallOrder: number[] } }) => fn.mock.invocationCallOrder;

test("Open, Close, Open with the first start pending: the new session survives", async () => {
  const d = deferred();
  StartSession.mockReturnValueOnce(d.promise);
  stopEmitsUserExit();
  render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(StartSession).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByRole("button", { name: "Close session" }));
  fireEvent.click(screen.getByRole("button", { name: "Open session" }));
  await waitFor(() => expect(fake.state.terms.length).toBe(2));
  // Nothing reaches the backend until the first start has settled.
  expect(StartSession).toHaveBeenCalledTimes(1);
  expect(StopSession).not.toHaveBeenCalled();
  await act(async () => {
    d.resolve();
    emit("terminal:data:m1", "first instance");
  });
  await waitFor(() => expect(StartSession).toHaveBeenCalledTimes(2));
  expect(StopSession).toHaveBeenCalledTimes(1);
  expect(order(StartSession)[0]).toBeLessThan(order(StopSession)[0]);
  expect(order(StopSession)[0]).toBeLessThan(order(StartSession)[1]);
  await act(async () => {});
  act(() => emit("terminal:data:m1", "second instance"));
  expect(screen.queryByText(/The session ended/)).toBeNull();
  expect(screen.getByRole("button", { name: "Close session" })).toBeInTheDocument();
  expect(lastTerm().written).toEqual(["second instance"]);
  expect(StopSession).toHaveBeenCalledTimes(1);
});

test("leave and reopen with the first start pending: the orphan is stopped and the new session survives", async () => {
  const d = deferred();
  StartSession.mockReturnValueOnce(d.promise);
  stopEmitsUserExit();
  const first = render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(StartSession).toHaveBeenCalledTimes(1));
  first.unmount();
  render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(fake.state.terms.length).toBe(2));
  expect(StartSession).toHaveBeenCalledTimes(1);
  await act(async () => d.resolve());
  await waitFor(() => expect(StartSession).toHaveBeenCalledTimes(2));
  expect(StopSession).toHaveBeenCalledTimes(1);
  expect(order(StopSession)[0]).toBeLessThan(order(StartSession)[1]);
  await act(async () => {});
  expect(screen.queryByText(/The session ended/)).toBeNull();
  expect(screen.getByRole("button", { name: "Close session" })).toBeInTheDocument();
});

test("a start that is closed before its turn never reaches the backend", async () => {
  const d = deferred();
  StartSession.mockReturnValueOnce(d.promise);
  render(<SessionCard meetingId="m1" hasSession />);
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(StartSession).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByRole("button", { name: "Close session" }));
  fireEvent.click(screen.getByRole("button", { name: "Open session" }));
  fireEvent.click(screen.getByRole("button", { name: "Close session" }));
  await act(async () => d.resolve());
  await act(async () => {});
  expect(StartSession).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "Open session" })).toBeInTheDocument();
});

const REVIEW_REASON = "Suggestions are being reviewed for this meeting. Open the session when the review finishes.";

test("a running review disables Open session with the review reason", async () => {
  render(<SessionCard meetingId="m1" hasSession reviewRunning />);
  expect(await screen.findByText(REVIEW_REASON)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Open session" })).toBeDisabled();
});

test("the no-session reason wins over the review reason", async () => {
  render(<SessionCard meetingId="m1" hasSession={false} reviewRunning />);
  expect(await screen.findByText("This meeting has no Claude Code session yet. Generate the summary first.")).toBeInTheDocument();
  expect(screen.queryByText(REVIEW_REASON)).toBeNull();
});

test("onOpenChange reports open, closed and unmount", async () => {
  const onOpenChange = vi.fn();
  const { unmount } = render(<SessionCard meetingId="m1" hasSession onOpenChange={onOpenChange} />);
  await waitFor(() => expect(onOpenChange).toHaveBeenLastCalledWith(false));
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(onOpenChange).toHaveBeenLastCalledWith(true));
  await waitFor(() => expect(fake.state.terms.length).toBe(1));
  fireEvent.click(screen.getByRole("button", { name: "Close session" }));
  await waitFor(() => expect(onOpenChange).toHaveBeenLastCalledWith(false));
  fireEvent.click(await screen.findByRole("button", { name: "Open session" }));
  await waitFor(() => expect(onOpenChange).toHaveBeenLastCalledWith(true));
  unmount();
  expect(onOpenChange).toHaveBeenLastCalledWith(false);
});
