import { act, fireEvent, render, screen } from "@testing-library/react";
import { PromptPreview } from "./PromptPreview";
import { PreviewPrompt, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

const choose = (key: string) => fireEvent.change(screen.getByLabelText("Preview the prompt for"), { target: { value: key } });

test("shows the header, the note and the placeholder, with the button off until a project is chosen", () => {
  render(<PromptPreview projects={["acme", "globex"]} projectsError="" />);
  expect(screen.getByText("Prompt preview")).toBeInTheDocument();
  expect(screen.getByText("This is exactly what the next summary for this project receives.")).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "Choose a project" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Preview prompt" })).toBeDisabled();
});

test("Preview prompt calls PreviewPrompt with the key and shows the text verbatim", async () => {
  const text = "  # Base\n\n  keep   the spacing\n<b>not html</b>\n";
  PreviewPrompt.mockResolvedValue(text);
  render(<PromptPreview projects={["acme", "globex"]} projectsError="" />);
  choose("acme");
  fireEvent.click(screen.getByRole("button", { name: "Preview prompt" }));
  const out = await screen.findByTestId("prompt-output");
  expect(PreviewPrompt).toHaveBeenCalledWith("acme");
  expect(out.textContent).toBe(text);
});

test("a second preview shows the updated text", async () => {
  PreviewPrompt.mockResolvedValueOnce("before the edit").mockResolvedValueOnce("after the edit");
  render(<PromptPreview projects={["acme"]} projectsError="" />);
  choose("acme");
  fireEvent.click(screen.getByRole("button", { name: "Preview prompt" }));
  expect((await screen.findByTestId("prompt-output")).textContent).toBe("before the edit");
  fireEvent.click(screen.getByRole("button", { name: "Preview prompt" }));
  await screen.findByText("after the edit");
});

test("a rejection is shown verbatim in an error card", async () => {
  PreviewPrompt.mockRejectedValue(new Error("There is no instructions/projects/acme.md in the archive."));
  render(<PromptPreview projects={["acme"]} projectsError="" />);
  choose("acme");
  fireEvent.click(screen.getByRole("button", { name: "Preview prompt" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("There is no instructions/projects/acme.md in the archive.");
  expect(screen.queryByTestId("prompt-output")).not.toBeInTheDocument();
});

test("a preview that resolves after another project was chosen is dropped", async () => {
  let resolveSlow!: (t: string) => void;
  PreviewPrompt.mockReturnValue(new Promise<string>((r) => (resolveSlow = r)));
  render(<PromptPreview projects={["acme", "globex"]} projectsError="" />);
  choose("acme");
  fireEvent.click(screen.getByRole("button", { name: "Preview prompt" }));
  choose("globex");
  await act(async () => resolveSlow("acme prompt"));
  expect(screen.queryByTestId("prompt-output")).not.toBeInTheDocument();
});

test("a preview that resolves after unmount does nothing", async () => {
  let resolveSlow!: (t: string) => void;
  PreviewPrompt.mockReturnValue(new Promise<string>((r) => (resolveSlow = r)));
  const spy = vi.spyOn(console, "error").mockImplementation(() => {});
  const { unmount } = render(<PromptPreview projects={["acme"]} projectsError="" />);
  choose("acme");
  fireEvent.click(screen.getByRole("button", { name: "Preview prompt" }));
  unmount();
  await act(async () => resolveSlow("late"));
  expect(spy).not.toHaveBeenCalled();
  spy.mockRestore();
});
