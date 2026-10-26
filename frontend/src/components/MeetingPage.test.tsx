import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MeetingPage } from "./MeetingPage";
import { EventsOn, GetDeletePlan, GetMeeting, GetReviewState, GetTranscript, ListOpenForms, ReviewSuggestions, StopRecording, emit, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));
vi.mock("@xterm/xterm", () => ({ Terminal: class { cols = 80; rows = 24; open() {} loadAddon() {} attachCustomKeyEventHandler() {} focus() {} write() {} onData() { return { dispose() {} }; } onResize() { return { dispose() {} }; } dispose() {} } }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));
beforeAll(() => { vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} unobserve() {} }); });

beforeEach(() => {
  resetBindings();
});
afterEach(() => vi.useRealTimers());

const seg = (id: number, track: string, startMs: number, text: string) => ({ id, track, startMs, endMs: startMs + 2000, text });

test("recording: pill ticks from startedAt and Stop recording stops", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  vi.setSystemTime(new Date("2026-10-23T14:05:05"));
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  await act(async () => {});
  expect(screen.getByText("Recording 00:05")).toBeInTheDocument();
  await act(async () => {
    vi.advanceTimersByTime(3000);
  });
  expect(screen.getByText("Recording 00:08")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Stop recording" }));
  expect(StopRecording).toHaveBeenCalledWith("m1");
});

test("transcribing shows the text and no stop button", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "transcribing" }));
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("Transcribing…")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Stop recording" })).not.toBeInTheDocument();
});

test("ready shows title and meta only", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByRole("heading", { level: 1, name: "Weekly sync" })).toBeInTheDocument();
  expect(screen.getByText("Oct 23, 2026, 14:05 · English")).toBeInTheDocument();
  expect(screen.queryByText("Transcribing…")).not.toBeInTheDocument();
});

test("warning and error cards show the message verbatim", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  await screen.findByRole("heading", { level: 1 });
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: "No sound from other people yet." }));
  expect(screen.getByText("Audio")).toBeInTheDocument();
  expect(screen.getByText("No sound from other people yet.")).toBeInTheDocument();
  await act(async () => emit("recording:state", { meetingId: "m1", state: "ready", error: "The recorder stopped." }));
  expect(screen.getByText("The recorder stopped.")).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent("The recorder stopped.");
});

test("a microphone warning and a system audio warning show together, each once", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  await screen.findByRole("heading", { level: 1 });
  const mic = "Your microphone recorded no sound (Studio Mic). If your MacBook's lid is closed, its built-in microphone is off.";
  const system = "No system audio was heard in the last minute.";
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: mic }));
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: system }));
  await act(async () => emit("recording:state", { meetingId: "m1", state: "recording", warning: mic }));
  expect(screen.getByText(mic)).toBeInTheDocument();
  expect(screen.getByText(system)).toBeInTheDocument();
  expect(screen.getAllByText(mic)).toHaveLength(1);
});

test("ignores state events of other meetings", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<MeetingPage meetingId="m1" />);
  await screen.findByRole("heading", { level: 1 });
  await act(async () => emit("recording:state", { meetingId: "other", state: "ready", error: "Not mine." }));
  expect(screen.queryByText("Not mine.")).not.toBeInTheDocument();
});

test("transcript: stored lines plus a live segment, deduped by id, with speaker labels", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  GetTranscript.mockResolvedValue([seg(1, "mic", 3000, "Hello everyone"), seg(2, "others", 65000, "Good morning")]);
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("Hello everyone")).toBeInTheDocument();
  expect(screen.getByText("00:00:03")).toBeInTheDocument();
  expect(screen.getByText("00:01:05")).toBeInTheDocument();
  expect(screen.getByText("You")).toBeInTheDocument();
  expect(screen.getByText("Others")).toBeInTheDocument();
  await act(async () => emit("transcript:segment", { meetingId: "m1", segment: seg(3, "mic", 70000, "Let us start") }));
  await act(async () => emit("transcript:segment", { meetingId: "m1", segment: seg(3, "mic", 70000, "Let us start") }));
  await act(async () => emit("transcript:segment", { meetingId: "other", segment: seg(9, "mic", 1000, "Elsewhere") }));
  expect(screen.getAllByText("Let us start")).toHaveLength(1);
  expect(screen.queryByText("Elsewhere")).not.toBeInTheDocument();
});

test("empty transcript text", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "recording" }));
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("The transcript appears here as each 30-second part is transcribed.")).toBeInTheDocument();
});

test("shows the model banner when the language's models are missing", async () => {
  const { GetModelStatus } = await import("../test/bindingsMock");
  GetModelStatus.mockResolvedValue({ language: "en", ready: false, missing: "x", downloadBytes: 100_000_000, downloading: false });
  GetMeeting.mockResolvedValue(meeting({ state: "transcribing" }));
  render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByText("Speech models")).toBeInTheDocument();
});

test("an open form shows the card under the header and leaves when the list empties", async () => {
  GetMeeting.mockResolvedValue(meeting());
  const f = { id: "f1", meetingId: "m1", kind: "ask", title: "Where next?", queued: 0, pieces: [{ kind: "text", id: "t", markdown: "hi", html: "", question: "", options: [], placeholder: "", multiline: false, required: false, yesLabel: "Yes", noLabel: "No" }] };
  ListOpenForms.mockResolvedValue([f] as any);
  const { container } = render(<MeetingPage meetingId="m1" />);
  expect(await screen.findByRole("region", { name: "Waiting for you" })).toBeInTheDocument();
  const head = container.querySelector("header")!;
  expect(head.nextElementSibling).toBe(screen.getByRole("region", { name: "Waiting for you" }));
  ListOpenForms.mockResolvedValue([] as any);
  await act(async () => emit("ask:changed", { meetingId: "m1" }));
  expect(screen.queryByRole("region", { name: "Waiting for you" })).not.toBeInTheDocument();
});

describe("review suggestions", () => {
  const withSuggestions = (over: Record<string, unknown> = {}) => meeting({ state: "summarized", hasSession: true, suggestionsMd: "# Suggestions\n\n- [ ] Send the draft", ...over });

  test("a review already running at load disables the button", async () => {
    GetReviewState.mockResolvedValue({ running: true });
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    expect(await screen.findByText("Reviewing suggestions… When Claude Code needs you, a form appears at the top of this page.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled();
    expect(GetReviewState).toHaveBeenCalledWith("m1");
  });

  test("a done event reloads the meeting once and shows the reply", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("button", { name: "Review suggestions" });
    const before = GetMeeting.mock.calls.length;
    await act(async () => emit("review:state", { meetingId: "m1", state: "done", message: "S-01 accepted" }));
    expect(await screen.findByText("Review finished.")).toBeInTheDocument();
    expect(screen.getByText("S-01 accepted")).toBeInTheDocument();
    expect(GetMeeting.mock.calls.length).toBe(before + 1);
  });

  test("an event for another meeting is ignored", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("button", { name: "Review suggestions" });
    await act(async () => emit("review:state", { meetingId: "other", state: "running" }));
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
  });

  test("clicking runs the review; a refusal shows its message", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    ReviewSuggestions.mockRejectedValue(new Error("Close the Claude Code session on this page first."));
    render(<MeetingPage meetingId="m1" />);
    fireEvent.click(await screen.findByRole("button", { name: "Review suggestions" }));
    expect(ReviewSuggestions).toHaveBeenCalledWith("m1");
    expect(await screen.findByText("Close the Claude Code session on this page first.")).toBeInTheDocument();
  });

  test("an open session disables the button, and closing it frees the button", async () => {
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    const open = await screen.findByRole("button", { name: "Open session" });
    await waitFor(() => expect(open).toBeEnabled());
    fireEvent.click(open);
    await waitFor(() => expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled());
    expect(screen.getByText("Close the Claude Code session below first. The review and the session use the same conversation.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close session" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled());
  });

  test("a running review disables Open session with its reason", async () => {
    GetReviewState.mockResolvedValue({ running: true });
    GetMeeting.mockResolvedValue(withSuggestions());
    render(<MeetingPage meetingId="m1" />);
    expect(await screen.findByText("Suggestions are being reviewed for this meeting. Open the session when the review finishes.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open session" })).toBeDisabled();
  });
});

describe("a summary pass blocks Review suggestions", () => {
  const REASON = "A summary is being written for this meeting. Review suggestions when it finishes.";
  const mk = (id: string) => meeting({ id, state: "summarized", summaryMd: "# Summary: Sync", suggestionsMd: "# Suggestions\n\n- [ ] Send the draft" });
  const sum = (meetingId: string, state: string, message?: string) => emit("summary:state", { meetingId, pass: "summary", state, message });
  const open = async () => {
    GetMeeting.mockImplementation(async (id: string) => mk(id));
    const r = render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("button", { name: "Review suggestions" });
    return r;
  };

  test("1: running disables the button, shows the reason and the Summary busy line", async () => {
    await open();
    await act(async () => sum("m1", "running"));
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled();
    expect(screen.getByText(REASON)).toBeInTheDocument();
    expect(screen.getByText("Claude Code is writing the summary…")).toBeInTheDocument();
  });

  test("2: another meeting's running changes nothing", async () => {
    await open();
    await act(async () => sum("m2", "running"));
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
    expect(screen.queryByText(REASON)).not.toBeInTheDocument();
  });

  test("3: done re-enables and reloads the meeting", async () => {
    await open();
    await act(async () => sum("m1", "running"));
    const before = GetMeeting.mock.calls.length;
    await act(async () => sum("m1", "done"));
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
    expect(screen.queryByText(REASON)).not.toBeInTheDocument();
    expect(GetMeeting.mock.calls.length).toBeGreaterThan(before);
  });

  test("4: error re-enables and the Summary card shows the message", async () => {
    await open();
    await act(async () => sum("m1", "running"));
    await act(async () => sum("m1", "error", "claude stopped"));
    expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
    expect(screen.queryByText(REASON)).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("claude stopped");
  });

  test("5: switching meetings resets the state", async () => {
    const { rerender } = await open();
    await act(async () => sum("m1", "running"));
    expect(screen.getByText(REASON)).toBeInTheDocument();
    rerender(<MeetingPage meetingId="m2" />);
    await waitFor(() => expect(GetMeeting).toHaveBeenCalledWith("m2"));
    expect(await screen.findByRole("button", { name: "Review suggestions" })).toBeEnabled();
    expect(screen.queryByText(REASON)).not.toBeInTheDocument();
  });

  test("6: the page holds one summary:state listener", async () => {
    await open();
    expect(EventsOn.mock.calls.filter(([n]) => n === "summary:state")).toHaveLength(1);
  });
});

describe("bringing a new form into view", () => {
  let scrolled: Element[];
  let calls: unknown[];
  let reduce: boolean;
  const mkForm = (id: string, meetingId = "m1") => ({ id, meetingId, kind: "ask", title: "Where next?", queued: 0, pieces: [{ kind: "text", id: "t", markdown: "hi", html: "", question: "", options: [], placeholder: "", multiline: false, required: false, yesLabel: "Yes", noLabel: "No" }] }) as any;
  const region = () => screen.getByRole("region", { name: "Waiting for you" });
  const arrive = async (...forms: unknown[]) => {
    ListOpenForms.mockResolvedValue(forms as any);
    await act(async () => emit("ask:changed", { meetingId: "m1" }));
  };

  beforeEach(() => {
    scrolled = [];
    calls = [];
    reduce = false;
    (window as any).matchMedia = (q: string) => ({ matches: reduce && q === "(prefers-reduced-motion: reduce)", media: q });
    Element.prototype.scrollIntoView = vi.fn(function (this: Element, arg?: unknown) {
      scrolled.push(this);
      calls.push(arg);
    });
    GetMeeting.mockImplementation(async (id: string) => meeting({ id }));
  });
  afterEach(() => {
    delete (window as any).matchMedia;
    delete (Element.prototype as any).scrollIntoView;
  });

  test("1. a form already open at load: no scroll, no focus", async () => {
    ListOpenForms.mockResolvedValue([mkForm("f1")] as any);
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("region", { name: "Waiting for you" });
    await act(async () => {});
    expect(scrolled).toHaveLength(0);
    expect(region()).not.toHaveFocus();
  });

  test("1b. the first list that arrives after the meeting rendered is not news", async () => {
    let release: (v: unknown[]) => void = () => {};
    ListOpenForms.mockReturnValue(new Promise((r) => (release = r)) as any);
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    await act(async () => release([mkForm("f1")]));
    await screen.findByRole("region", { name: "Waiting for you" });
    expect(scrolled).toHaveLength(0);
    expect(region()).not.toHaveFocus();
  });

  test("2. a new form while viewing scrolls smoothly to the card and focuses it", async () => {
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    const focusSpy = vi.spyOn(HTMLElement.prototype, "focus");
    try {
      await arrive(mkForm("f1"));
      expect(scrolled).toHaveLength(1);
      expect(scrolled[0]).toBe(region());
      expect(calls[0]).toEqual({ behavior: "smooth", block: "nearest" });
      expect(region()).toHaveFocus();
      const i = focusSpy.mock.contexts.indexOf(region());
      expect(i).toBeGreaterThanOrEqual(0);
      expect(focusSpy.mock.calls[i][0]).toEqual({ preventScroll: true });
    } finally {
      focusSpy.mockRestore();
    }
  });

  test("3. reduced motion scrolls at once", async () => {
    reduce = true;
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    await arrive(mkForm("f1"));
    expect(calls).toEqual([{ behavior: "auto", block: "nearest" }]);
  });

  test("4. the next queued form is brought into view too", async () => {
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    await arrive(mkForm("f1"));
    await arrive(mkForm("f2"));
    expect(scrolled).toHaveLength(2);
    expect(scrolled[1]).toBe(region());
    expect(region()).toHaveFocus();
  });

  test("5. a reload or re-render with the same form does not scroll again", async () => {
    render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    await arrive(mkForm("f1"));
    await arrive(mkForm("f1"));
    await act(async () => emit("recording:state", { meetingId: "m1", state: "ready" }));
    await act(async () => emit("review:state", { meetingId: "m1", state: "done", message: "" }));
    expect(scrolled).toHaveLength(1);
  });

  test("6. navigating to a meeting whose form is already open does not scroll", async () => {
    const { rerender } = render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    ListOpenForms.mockResolvedValue([mkForm("f3", "m2")] as any);
    await act(async () => emit("ask:changed", { meetingId: "m2" }));
    rerender(<MeetingPage meetingId="m2" />);
    await screen.findByRole("region", { name: "Waiting for you" });
    await act(async () => {});
    expect(scrolled).toHaveLength(0);
    expect(region()).not.toHaveFocus();
  });

  test("7. focus stays in the terminal, the page still scrolls", async () => {
    GetMeeting.mockImplementation(async (id: string) => meeting({ id, state: "summarized", hasSession: true, suggestionsMd: "# S" }));
    render(<MeetingPage meetingId="m1" />);
    const open = await screen.findByRole("button", { name: "Open session" });
    await waitFor(() => expect(open).toBeEnabled());
    fireEvent.click(open);
    const term = await screen.findByTestId("terminal");
    const ta = document.createElement("textarea");
    term.appendChild(ta);
    ta.focus();
    await arrive(mkForm("f1"));
    expect(scrolled).toHaveLength(1);
    expect(ta).toHaveFocus();
  });

  test("8. focus stays in a dialog, the page still scrolls", async () => {
    GetDeletePlan.mockResolvedValue({ audioFiles: 1, archiveFolder: "" });
    render(<MeetingPage meetingId="m1" />);
    fireEvent.click(await screen.findByRole("button", { name: "Delete meeting" }));
    await screen.findByRole("dialog");
    await arrive(mkForm("f1"));
    expect(scrolled).toHaveLength(1);
    expect(document.activeElement!.closest('[role="dialog"]')).not.toBeNull();
  });

  test("9. a notification click reveals the card of a form already open, once", async () => {
    const { rerender } = render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    ListOpenForms.mockResolvedValue([mkForm("f3", "m2")] as any);
    await act(async () => emit("ask:changed", { meetingId: "m2" }));
    rerender(<MeetingPage meetingId="m2" revealSeq={1} />);
    await screen.findByRole("region", { name: "Waiting for you" });
    await act(async () => {});
    expect(scrolled).toHaveLength(1);
    expect(scrolled[0]).toBe(region());
    expect(region()).toHaveFocus();
    rerender(<MeetingPage meetingId="m2" revealSeq={1} />);
    await act(async () => emit("ask:changed", { meetingId: "m2" }));
    expect(scrolled).toHaveLength(1);
  });

  test("9b. a handled reveal number is not used for a later meeting", async () => {
    ListOpenForms.mockResolvedValue([mkForm("f3", "m2"), mkForm("f5", "m5")] as any);
    const { rerender } = render(<MeetingPage meetingId="m2" revealSeq={1} />);
    await screen.findByRole("region", { name: "Waiting for you" });
    await act(async () => {});
    expect(scrolled).toHaveLength(1);
    rerender(<MeetingPage meetingId="m5" revealSeq={1} />);
    await act(async () => {});
    await screen.findByRole("region", { name: "Waiting for you" });
    expect(scrolled).toHaveLength(1);
  });

  test("10. a second notification click for the meeting on screen reveals again", async () => {
    ListOpenForms.mockResolvedValue([mkForm("f3", "m2")] as any);
    const { rerender } = render(<MeetingPage meetingId="m2" revealSeq={1} />);
    await screen.findByRole("region", { name: "Waiting for you" });
    await act(async () => {});
    expect(scrolled).toHaveLength(1);
    rerender(<MeetingPage meetingId="m2" revealSeq={2} />);
    await act(async () => {});
    expect(scrolled).toHaveLength(2);
  });

  test("10b. dropping the reveal number while the same form stays open does not scroll again", async () => {
    ListOpenForms.mockResolvedValue([mkForm("f3", "m2")] as any);
    const { rerender } = render(<MeetingPage meetingId="m2" revealSeq={1} />);
    await screen.findByRole("region", { name: "Waiting for you" });
    await act(async () => {});
    expect(scrolled).toHaveLength(1);
    rerender(<MeetingPage meetingId="m2" />);
    await act(async () => {});
    expect(scrolled).toHaveLength(1);
  });

  test("11. a notification click with no open form does nothing, and is not kept for later", async () => {
    const { rerender } = render(<MeetingPage meetingId="m1" />);
    await screen.findByRole("heading", { level: 1 });
    rerender(<MeetingPage meetingId="m2" revealSeq={1} />);
    await screen.findByRole("heading", { level: 1 });
    await act(async () => {});
    expect(scrolled).toHaveLength(0);
    ListOpenForms.mockResolvedValue([mkForm("f3", "m2")] as any);
    await act(async () => emit("ask:changed", { meetingId: "m2" }));
    expect(scrolled).toHaveLength(1);
  });
});
