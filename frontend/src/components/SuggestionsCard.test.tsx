import { fireEvent, render, screen } from "@testing-library/react";
import { SuggestionsCard } from "./SuggestionsCard";

type Props = Parameters<typeof SuggestionsCard>[0];
const base: Props = { suggestionsMd: "# Suggestions\n\n- [ ] Send the draft", phase: "idle", message: "", sessionOpen: false, summaryRunning: false, onReview: () => {} };
const show = (over: Partial<Props> = {}) => render(<SuggestionsCard {...base} {...over} />);

test("no suggestions: the old text and no button", () => {
  show({ suggestionsMd: "" });
  expect(screen.getByText("No suggestions yet.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Review suggestions" })).not.toBeInTheDocument();
});

test("idle: enabled button with the helper note", () => {
  show();
  expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
  expect(screen.getByText("Claude Code shows the suggestions as a form at the top of this page. Nothing changes until you answer.")).toBeInTheDocument();
});

test("clicking calls onReview", () => {
  const onReview = vi.fn();
  show({ onReview });
  fireEvent.click(screen.getByRole("button", { name: "Review suggestions" }));
  expect(onReview).toHaveBeenCalledTimes(1);
});

test("running: disabled with the running note", () => {
  show({ phase: "running" });
  expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled();
  expect(screen.getByText("Reviewing suggestions… When Claude Code needs you, a form appears at the top of this page.")).toBeInTheDocument();
});

test("session open: disabled with the close-first note", () => {
  show({ sessionOpen: true });
  expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled();
  expect(screen.getByText("Close the Claude Code session below first. The review and the session use the same conversation.")).toBeInTheDocument();
});

test("running wins over session open", () => {
  show({ phase: "running", sessionOpen: true });
  expect(screen.getByText(/^Reviewing suggestions…/)).toBeInTheDocument();
  expect(screen.queryByText(/^Close the Claude Code session below first/)).not.toBeInTheDocument();
});

test("done: finished note and the reply, rendered untrusted", () => {
  const { container } = show({ phase: "done", message: "S-01 accepted\n\n<script>x</script> [a link](other.md)" });
  expect(screen.getByText("Review finished.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
  const reply = container.querySelector(".nt-review__reply")!;
  expect(reply).toHaveTextContent("S-01 accepted");
  expect(reply.querySelector("script")).toBeNull();
  expect(reply.querySelector("a")).toBeNull();
});

test("done with an empty reply: no reply block", () => {
  const { container } = show({ phase: "done", message: "" });
  expect(screen.getByText("Review finished.")).toBeInTheDocument();
  expect(container.querySelector(".nt-review__reply")).toBeNull();
});

test("error: alert with the message and no note", () => {
  show({ phase: "error", message: "Close the Claude Code session on this page first." });
  expect(screen.getByRole("alert")).toHaveTextContent("Close the Claude Code session on this page first.");
  expect(screen.queryByText(/^Claude Code shows the suggestions here/)).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Review suggestions" })).toBeEnabled();
});

test("done: the reply's Markdown sits inside the reply block", () => {
  const { container } = show({ phase: "done", message: "Done:\n\n```\nhook output\n```" });
  const reply = container.querySelector(".nt-review__reply")!;
  expect(reply.parentElement).toBe(container.querySelector(".nt-review"));
  expect(reply.querySelector(".nt-md pre")).not.toBeNull();
});

const REASON = "A summary is being written for this meeting. Review suggestions when it finishes.";

test("summary running: disabled with the summary reason", () => {
  show({ summaryRunning: true });
  expect(screen.getByRole("button", { name: "Review suggestions" })).toBeDisabled();
  expect(screen.getByText(REASON)).toBeInTheDocument();
});

test("review running wins over summary running", () => {
  show({ phase: "running", summaryRunning: true });
  expect(screen.getByText(/^Reviewing suggestions…/)).toBeInTheDocument();
  expect(screen.queryByText(REASON)).not.toBeInTheDocument();
});

test("summary running wins over session open, done and error", () => {
  const cases: Partial<Props>[] = [{ sessionOpen: true }, { phase: "done", message: "S-01 accepted" }, { phase: "error", message: "nope" }];
  for (const c of cases) {
    const { unmount } = show({ summaryRunning: true, ...c });
    expect(screen.getByText(REASON)).toBeInTheDocument();
    expect(screen.queryByText(/^Close the Claude Code session below first/)).not.toBeInTheDocument();
    expect(screen.queryByText("Review finished.")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    unmount();
  }
});
