import { fireEvent, render, screen } from "@testing-library/react";
import { ConfirmDialog } from "./ConfirmDialog";

function setup(over: Record<string, unknown> = {}) {
  const props = { title: "Delete profile bruno?", body: "Notted deletes these files:", cancelLabel: "Keep profile", confirmLabel: "Delete profile", onCancel: vi.fn(), onConfirm: vi.fn(), ...over };
  render(<ConfirmDialog {...(props as any)} />);
  return props;
}

test("shows the title, the body, every item in the given order and the hint", () => {
  setup({ items: ["b.md", "a/", "a/c.md"], hint: "Nothing is committed." });
  expect(screen.getByRole("dialog", { name: "Delete profile bruno?" })).toBeInTheDocument();
  expect(screen.getByText("Notted deletes these files:")).toBeInTheDocument();
  expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["b.md", "a/", "a/c.md"]);
  expect(screen.getByText("Nothing is committed.")).toHaveClass("nt-dialog__hint");
});

test("renders no list and no hint when the props are absent or empty", () => {
  setup({ items: [] });
  expect(screen.queryByRole("list")).not.toBeInTheDocument();
  expect(document.querySelector(".nt-dialog__hint")).toBeNull();
});

test("the cancel button has focus on open", () => {
  setup();
  expect(screen.getByRole("button", { name: "Keep profile" })).toHaveFocus();
});

test("Escape calls onCancel and the confirm button calls onConfirm", () => {
  const p = setup();
  fireEvent.keyDown(document, { key: "Escape" });
  expect(p.onCancel).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Delete profile" }));
  expect(p.onConfirm).toHaveBeenCalledTimes(1);
});

test("destructive makes the confirm button red; without it the button is the primary one", () => {
  setup({ destructive: true });
  expect(screen.getByRole("button", { name: "Delete profile" })).toHaveClass("nt-btn--danger");
});

test("a confirm that is not destructive keeps the primary style", () => {
  setup();
  const b = screen.getByRole("button", { name: "Delete profile" });
  expect(b).toHaveClass("nt-btn--primary");
  expect(b).not.toHaveClass("nt-btn--danger");
});

test("busy disables both buttons, Escape does not cancel and clicks do nothing", () => {
  const p = setup({ busy: true });
  expect(screen.getByRole("button", { name: "Keep profile" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Delete profile" })).toBeDisabled();
  fireEvent.keyDown(document, { key: "Escape" });
  fireEvent.click(screen.getByRole("button", { name: "Keep profile" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete profile" }));
  expect(p.onCancel).not.toHaveBeenCalled();
  expect(p.onConfirm).not.toHaveBeenCalled();
});

test("without busy both buttons are enabled", () => {
  setup();
  expect(screen.getByRole("button", { name: "Keep profile" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Delete profile" })).toBeEnabled();
});

test("error renders an alert after the hint; no alert without error", () => {
  setup({ hint: "A hint.", error: "boom" });
  const alert = screen.getByRole("alert");
  expect(alert).toHaveTextContent("boom");
  expect(alert).toHaveClass("nt-dialog__error");
  expect(screen.getByText("A hint.").compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test("no alert is rendered without an error", () => {
  setup({ hint: "A hint." });
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
