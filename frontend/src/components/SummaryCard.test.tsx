import { act, fireEvent, render, screen } from "@testing-library/react";
import { useCallback, useEffect, useState } from "react";
import { SummaryCard } from "./SummaryCard";
import { SuggestionsCard } from "./SuggestionsCard";
import { useSummaryRun } from "../summary";
import { GenerateSummary, GetMeeting, emit, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

function Harness({ notCommitted = false }: { notCommitted?: boolean }) {
  const [m, setM] = useState<any>(null);
  const refresh = useCallback(() => void GetMeeting("m1").then(setM), []);
  useEffect(refresh, [refresh]);
  const run = useSummaryRun("m1", refresh);
  if (!m) return null;
  return (
    <>
      <SummaryCard meeting={m} notCommitted={notCommitted} run={run} />
      <SuggestionsCard suggestionsMd={m.suggestionsMd} phase="idle" message="" sessionOpen={false} summaryRunning={run.pass !== null} onReview={() => {}} />
    </>
  );
}

test("disabled with a reason while the transcript is not finished", async () => {
  for (const state of ["recording", "transcribing"]) {
    GetMeeting.mockResolvedValue(meeting({ state }));
    const { unmount } = render(<Harness />);
    expect(await screen.findByRole("button", { name: "Generate summary" })).toBeDisabled();
    expect(screen.getByText("Available when the transcript is finished.")).toBeInTheDocument();
    unmount();
  }
});

test("disabled with a reason when there is no project", async () => {
  GetMeeting.mockResolvedValue(meeting({ projectKey: "" }));
  render(<Harness />);
  expect(await screen.findByRole("button", { name: "Generate summary" })).toBeDisabled();
  expect(screen.getByText("Choose a project first.")).toBeInTheDocument();
});

test("click calls GenerateSummary", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<Harness />);
  const btn = await screen.findByRole("button", { name: "Generate summary" });
  expect(btn).toBeEnabled();
  fireEvent.click(btn);
  expect(GenerateSummary).toHaveBeenCalledWith("m1");
});

test("a rejection shows its message verbatim in an alert", async () => {
  GenerateSummary.mockRejectedValue(new Error("Claude Code is not installed."));
  GetMeeting.mockResolvedValue(meeting());
  render(<Harness />);
  fireEvent.click(await screen.findByRole("button", { name: "Generate summary" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Claude Code is not installed.");
});

test("running texts per pass disable the buttons", async () => {
  GetMeeting.mockResolvedValue(meeting({ state: "summarized", summaryMd: "# Summary: Sync" }));
  render(<Harness />);
  await screen.findByRole("button", { name: "Generate again" });
  await act(async () => emit("summary:state", { meetingId: "m1", pass: "summary", state: "running" }));
  expect(screen.getByText("Claude Code is writing the summary…")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Generate again" })).toBeDisabled();
  await act(async () => emit("summary:state", { meetingId: "m1", pass: "suggestions", state: "running" }));
  expect(screen.getByText("Claude Code is writing the suggestions…")).toBeInTheDocument();
  expect(screen.queryByText("Claude Code is writing the summary…")).not.toBeInTheDocument();
});

test("an error event shows its message verbatim and re-enables", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<Harness />);
  await screen.findByRole("button", { name: "Generate summary" });
  await act(async () => emit("summary:state", { meetingId: "m1", pass: "summary", state: "running" }));
  await act(async () => emit("summary:state", { meetingId: "m1", pass: "summary", state: "error", message: "claude exited with code 1:\nlogin required" }));
  expect(screen.getByRole("alert")).toHaveTextContent("claude exited with code 1:");
  expect(screen.getByRole("button", { name: "Generate summary" })).toBeEnabled();
});

test("events of other meetings are ignored", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<Harness />);
  await screen.findByRole("button", { name: "Generate summary" });
  await act(async () => emit("summary:state", { meetingId: "other", pass: "summary", state: "running" }));
  expect(screen.queryByText("Claude Code is writing the summary…")).not.toBeInTheDocument();
});

test("after done the stored Markdown is read again and rendered", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<Harness />);
  await screen.findByRole("button", { name: "Generate summary" });
  expect(screen.getByText("No suggestions yet.")).toBeInTheDocument();
  GetMeeting.mockResolvedValue(
    meeting({ state: "summarized", summaryMd: "# Summary: Weekly sync\n\n- Decided to ship", suggestionsMd: "# Suggestions\n\n- [ ] Send the draft" }),
  );
  await act(async () => emit("summary:state", { meetingId: "m1", pass: "suggestions", state: "done" }));
  expect(await screen.findByRole("heading", { name: "Summary: Weekly sync" })).toBeInTheDocument();
  expect(screen.getByText("Decided to ship")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Generate again" })).toBeEnabled();
  expect(screen.getByText("Send the draft")).toBeInTheDocument();
  expect(screen.queryByText("No suggestions yet.")).not.toBeInTheDocument();
});

test("the not-committed footer", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<Harness notCommitted />);
  expect(await screen.findByText("The files are in the archive folder, not committed.")).toBeInTheDocument();
});
