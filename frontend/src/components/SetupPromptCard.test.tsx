import { act, fireEvent, render, screen } from "@testing-library/react";
import { SetupPromptCard } from "./SetupPromptCard";
import { ClipboardSetText, GetSetupPrompt, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);
afterEach(() => vi.useRealTimers());

const copyButton = () => screen.getByRole("button", { name: "Copy setup prompt" });

test("shows the header, the note, both buttons and an empty status", () => {
  render(<SetupPromptCard />);
  expect(screen.getByText("Set up with Claude Code")).toBeInTheDocument();
  expect(copyButton()).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Show prompt" })).toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("");
  expect(screen.queryByTestId("setup-prompt-output")).not.toBeInTheDocument();
});

test("Copy setup prompt copies the text, announces it and keeps focus on the button", async () => {
  render(<SetupPromptCard />);
  copyButton().focus();
  fireEvent.click(copyButton());
  expect(await screen.findByText("Copied. Paste it into Claude Code.")).toBeInTheDocument();
  expect(ClipboardSetText).toHaveBeenCalledWith("SETUP PROMPT TEXT");
  expect(screen.getByRole("status")).toHaveTextContent("Copied. Paste it into Claude Code.");
  expect(copyButton()).toHaveFocus();
  expect(screen.queryByTestId("setup-prompt-output")).not.toBeInTheDocument();
});

test("the copied message goes away after 4 seconds", async () => {
  vi.useFakeTimers();
  render(<SetupPromptCard />);
  await act(async () => {
    fireEvent.click(copyButton());
  });
  expect(screen.getByRole("status")).toHaveTextContent("Copied.");
  act(() => {
    vi.advanceTimersByTime(3900);
  });
  expect(screen.getByRole("status")).toHaveTextContent("Copied.");
  act(() => {
    vi.advanceTimersByTime(200);
  });
  expect(screen.getByRole("status")).toHaveTextContent("");
});

test("a clipboard that answers false shows Could not copy and the prompt in a read-only box", async () => {
  ClipboardSetText.mockResolvedValue(false);
  render(<SetupPromptCard />);
  fireEvent.click(copyButton());
  expect(await screen.findByText("Could not copy.")).toBeInTheDocument();
  const box = screen.getByTestId("setup-prompt-output") as HTMLTextAreaElement;
  expect(box.value).toBe("SETUP PROMPT TEXT");
  expect(box).toHaveAttribute("readonly");
});

test("a clipboard that rejects shows the same fallback", async () => {
  ClipboardSetText.mockRejectedValue(new Error("no clipboard"));
  render(<SetupPromptCard />);
  fireEvent.click(copyButton());
  expect(await screen.findByText("Could not copy.")).toBeInTheDocument();
  expect((screen.getByTestId("setup-prompt-output") as HTMLTextAreaElement).value).toBe("SETUP PROMPT TEXT");
});

test("a prompt that cannot be built shows its error and Could not copy", async () => {
  GetSetupPrompt.mockRejectedValue(new Error("settings unreadable"));
  render(<SetupPromptCard />);
  fireEvent.click(copyButton());
  expect(await screen.findByRole("alert")).toHaveTextContent("settings unreadable");
  expect(screen.getByRole("status")).toHaveTextContent("Could not copy.");
  expect(ClipboardSetText).not.toHaveBeenCalled();
});

test("Show prompt reveals the text verbatim and Hide prompt hides it", async () => {
  GetSetupPrompt.mockResolvedValue("  line one\n\n  line <b>two</b>\n");
  render(<SetupPromptCard />);
  fireEvent.click(screen.getByRole("button", { name: "Show prompt" }));
  const box = (await screen.findByTestId("setup-prompt-output")) as HTMLTextAreaElement;
  expect(box.value).toBe("  line one\n\n  line <b>two</b>\n");
  expect(ClipboardSetText).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Hide prompt" }));
  expect(screen.queryByTestId("setup-prompt-output")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Show prompt" }));
  expect(await screen.findByTestId("setup-prompt-output")).toBeInTheDocument();
  expect(GetSetupPrompt).toHaveBeenCalledTimes(2);
});

test("Show prompt reports an error from the binding", async () => {
  GetSetupPrompt.mockRejectedValue(new Error("settings unreadable"));
  render(<SetupPromptCard />);
  fireEvent.click(screen.getByRole("button", { name: "Show prompt" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("settings unreadable");
  expect(screen.queryByTestId("setup-prompt-output")).not.toBeInTheDocument();
});

test("an answer that arrives after unmount does nothing", async () => {
  let resolveSlow!: (t: string) => void;
  GetSetupPrompt.mockReturnValue(new Promise<string>((r) => (resolveSlow = r)));
  const spy = vi.spyOn(console, "error").mockImplementation(() => {});
  const { unmount } = render(<SetupPromptCard />);
  fireEvent.click(copyButton());
  unmount();
  await act(async () => resolveSlow("late"));
  expect(ClipboardSetText).not.toHaveBeenCalled();
  expect(spy).not.toHaveBeenCalled();
  spy.mockRestore();
});

test("a second Copy starts a fresh message", async () => {
  render(<SetupPromptCard />);
  fireEvent.click(copyButton());
  await screen.findByText("Copied. Paste it into Claude Code.");
  ClipboardSetText.mockResolvedValue(false);
  fireEvent.click(copyButton());
  expect(await screen.findByText("Could not copy.")).toBeInTheDocument();
  expect(screen.queryByText("Copied. Paste it into Claude Code.")).not.toBeInTheDocument();
});

test("Show prompt fetches again each time, so a changed state shows the new text", async () => {
  GetSetupPrompt.mockResolvedValueOnce("Archive folder: not chosen yet").mockResolvedValueOnce("Archive folder: /archive");
  render(<SetupPromptCard />);
  fireEvent.click(screen.getByRole("button", { name: "Show prompt" }));
  expect(((await screen.findByTestId("setup-prompt-output")) as HTMLTextAreaElement).value).toBe("Archive folder: not chosen yet");
  fireEvent.click(screen.getByRole("button", { name: "Hide prompt" }));
  fireEvent.click(screen.getByRole("button", { name: "Show prompt" }));
  await screen.findByDisplayValue("Archive folder: /archive");
});

test("pressing Show prompt while a Copy is pending does not cancel the Copy", async () => {
  let resolveCopy!: (t: string) => void;
  GetSetupPrompt.mockReturnValueOnce(new Promise<string>((r) => (resolveCopy = r))).mockResolvedValue("shown text");
  render(<SetupPromptCard />);
  fireEvent.click(copyButton());
  fireEvent.click(screen.getByRole("button", { name: "Show prompt" }));
  await screen.findByDisplayValue("shown text");
  await act(async () => resolveCopy("copied text"));
  expect(ClipboardSetText).toHaveBeenCalledWith("copied text");
  expect(screen.getByRole("status")).toHaveTextContent("Copied. Paste it into Claude Code.");
});
