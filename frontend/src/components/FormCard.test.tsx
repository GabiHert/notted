import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { FormCard } from "./FormCard";
import { AnswerForm, DismissForm, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(() => resetBindings());

const piece = (over: Record<string, unknown>) => ({
  kind: "text", id: "p", markdown: "", html: "", question: "", options: [], placeholder: "",
  multiline: false, required: false, yesLabel: "Yes", noLabel: "No", ...over,
});
const opt = (label: string, description = "", recommended = false) => ({ label, description, recommended });
const form = (pieces: unknown[], over: Record<string, unknown> = {}) =>
  ({ id: "f1", meetingId: "m1", kind: "ask", title: "A question", pieces, queued: 0, ...over }) as any;

const choice = piece({ kind: "choice", id: "where", question: "Pick the folder", options: [opt("tasks/", "Main", true), opt("notes/")] });
const multi = piece({ kind: "multi", id: "accept", question: "Which?", options: [opt("S-01"), opt("S-02"), opt("S-03")] });
const input = piece({ kind: "input", id: "owner", question: "Who owns it?", placeholder: "A name", required: true });
const optInput = piece({ kind: "input", id: "why", question: "Why?", multiline: true });
const confirm = piece({ kind: "confirm", id: "go", question: "Write this file?", yesLabel: "Write it", noLabel: "Not this one" });

test("header, title and permission header", () => {
  const { rerender } = render(<FormCard form={form([choice])} />);
  expect(screen.getByRole("region", { name: "Waiting for you" })).toBeInTheDocument();
  expect(screen.getByText("Waiting for you")).toBeInTheDocument();
  expect(screen.getByRole("heading", { level: 2, name: "A question" })).toBeInTheDocument();
  rerender(<FormCard form={form([confirm], { kind: "permission" })} />);
  expect(screen.getByText("Waiting for you · Permission")).toBeInTheDocument();
});

test("queue line for 0, 1 and 3", () => {
  const { rerender } = render(<FormCard form={form([choice])} />);
  expect(screen.queryByText(/waiting after this one/)).not.toBeInTheDocument();
  rerender(<FormCard form={form([choice], { queued: 1 })} />);
  expect(screen.getByText("1 more form is waiting after this one.")).toBeInTheDocument();
  rerender(<FormCard form={form([choice], { queued: 3 })} />);
  expect(screen.getByText("3 more forms are waiting after this one.")).toBeInTheDocument();
});

test("renders each piece kind", () => {
  render(<FormCard form={form([choice, multi, input, optInput, piece({ kind: "text", id: "t", markdown: "hello **there**" }), confirm])} />);
  expect(screen.getAllByRole("radio")).toHaveLength(2);
  expect(screen.getAllByRole("checkbox")).toHaveLength(3);
  expect(screen.getByText("Recommended")).toBeInTheDocument();
  expect(screen.getByText("Main")).toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Who owns it? (required)" })).toHaveAttribute("placeholder", "A name");
  expect(screen.getByRole("textbox", { name: "Why?" }).tagName).toBe("TEXTAREA");
  expect(screen.getByRole("button", { name: "Write it" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Not this one" })).toBeInTheDocument();
  expect(screen.getByText("there").tagName).toBe("STRONG");
  expect(screen.getByLabelText("Note (optional)")).toHaveAttribute("placeholder", "Anything Claude should know");
  for (const r of screen.getAllByRole("radio")) expect(r).not.toBeChecked();
});

test("html piece renders a sandboxed frame", () => {
  const { container } = render(<FormCard form={form([piece({ kind: "html", id: "h", html: "<p>x</p>" })])} />);
  expect(container.querySelector("iframe")!.getAttribute("sandbox")).toBe("");
});

test("Send stays disabled until choice, confirm and required input are set", () => {
  render(<FormCard form={form([choice, input, confirm])} />);
  const send = screen.getByRole("button", { name: "Send" });
  expect(send).toBeDisabled();
  fireEvent.click(screen.getByRole("radio", { name: /tasks\// }));
  expect(send).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Write it" }));
  expect(send).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox", { name: "Who owns it? (required)" }), { target: { value: "   " } });
  expect(send).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox", { name: "Who owns it? (required)" }), { target: { value: "me" } });
  expect(send).toBeEnabled();
  expect(screen.getByRole("button", { name: "Write it" })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("button", { name: "Not this one" })).toHaveAttribute("aria-pressed", "false");
});

test("Send sends the exact payload", async () => {
  render(<FormCard form={form([choice, multi, input, optInput, confirm])} />);
  fireEvent.click(screen.getByRole("radio", { name: /notes\// }));
  fireEvent.click(screen.getByRole("checkbox", { name: "S-03" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "S-01" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Who owns it? (required)" }), { target: { value: " Ana " } });
  fireEvent.click(screen.getByRole("button", { name: "Not this one" }));
  fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "ok" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(AnswerForm).toHaveBeenCalled());
  expect(AnswerForm).toHaveBeenCalledWith("f1", { where: "notes/", accept: ["S-01", "S-03"], owner: " Ana ", why: "", go: "no" }, "ok");
});

test("an empty multi sends []", async () => {
  render(<FormCard form={form([multi])} />);
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(AnswerForm).toHaveBeenCalledWith("f1", { accept: [] }, ""));
});

test("text and html pieces are never in answers", async () => {
  render(<FormCard form={form([piece({ kind: "text", id: "t", markdown: "x" }), piece({ kind: "html", id: "h", html: "y" }), multi])} />);
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(AnswerForm).toHaveBeenCalledWith("f1", { accept: [] }, ""));
});

test("a confirm-only form has no Send and answers on click", async () => {
  render(<FormCard form={form([piece({ kind: "text", id: "t", markdown: "x" }), confirm])} />);
  expect(screen.queryByRole("button", { name: "Send" })).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "n" } });
  fireEvent.click(screen.getByRole("button", { name: "Write it" }));
  await waitFor(() => expect(AnswerForm).toHaveBeenCalledWith("f1", { go: "yes" }, "n"));
});

test("Not now dismisses with the note", async () => {
  render(<FormCard form={form([choice])} />);
  fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "later" } });
  fireEvent.click(screen.getByRole("button", { name: "Not now" }));
  await waitFor(() => expect(DismissForm).toHaveBeenCalledWith("f1", "later"));
});

test("controls are disabled while a call is pending", async () => {
  let release: () => void = () => {};
  AnswerForm.mockReturnValue(new Promise<void>((r) => (release = r)));
  render(<FormCard form={form([choice])} />);
  fireEvent.click(screen.getByRole("radio", { name: /tasks\// }));
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled());
  expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  expect(screen.getByLabelText("Note (optional)")).toBeDisabled();
  expect(screen.getAllByRole("radio")[0]).toBeDisabled();
  await act(async () => release());
  expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled();
});

test("a failed call shows an alert and re-enables", async () => {
  AnswerForm.mockRejectedValue(new Error("The session already ended."));
  render(<FormCard form={form([choice])} />);
  fireEvent.click(screen.getByRole("radio", { name: /tasks\// }));
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("The session already ended.");
  expect(screen.getByRole("button", { name: "Send" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Not now" })).toBeEnabled();
});
