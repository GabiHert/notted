import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { ModelsCard } from "./ModelsCard";
import { GetUnusedModels, RemoveUnusedModel, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

const old = [
  { name: "sherpa-onnx-whisper-small", bytes: 1_300_000_000 },
  { name: "sherpa-onnx-whisper-turbo", bytes: 989_000_000 },
  { name: "silero_vad", bytes: 644_000 },
];

test("nothing unused: the card is not shown", async () => {
  const { container } = render(<ModelsCard />);
  await waitFor(() => expect(GetUnusedModels).toHaveBeenCalled());
  expect(container).toBeEmptyDOMElement();
});

test("lists every unused model with its size and the total", async () => {
  GetUnusedModels.mockResolvedValue(old);
  render(<ModelsCard />);
  expect(await screen.findByText("Unused speech models")).toBeInTheDocument();
  expect(screen.getByText("Earlier versions of Phonon used these models. Nothing uses them now; together they take 2.3 GB.")).toBeInTheDocument();
  const rows = screen.getAllByRole("listitem");
  expect(rows.map((r) => r.textContent)).toEqual(["sherpa-onnx-whisper-small1.3 GBRemove", "sherpa-onnx-whisper-turbo989 MBRemove", "silero_vad644 KBRemove"]);
  expect(RemoveUnusedModel).not.toHaveBeenCalled();
});

test("Remove asks first; Cancel removes nothing", async () => {
  GetUnusedModels.mockResolvedValue(old);
  render(<ModelsCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Remove sherpa-onnx-whisper-turbo" }));
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText("Remove sherpa-onnx-whisper-turbo?")).toBeInTheDocument();
  expect(within(dialog).getByText("This deletes sherpa-onnx-whisper-turbo (989 MB) from the models folder. Phonon does not use it.")).toBeInTheDocument();
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(RemoveUnusedModel).not.toHaveBeenCalled();
});

test("confirming removes that one model and reloads the list", async () => {
  GetUnusedModels.mockResolvedValueOnce(old).mockResolvedValueOnce(old.slice(0, 1).concat(old.slice(2)));
  render(<ModelsCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Remove sherpa-onnx-whisper-turbo" }));
  await act(async () => {
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }));
  });
  expect(RemoveUnusedModel).toHaveBeenCalledTimes(1);
  expect(RemoveUnusedModel).toHaveBeenCalledWith("sherpa-onnx-whisper-turbo");
  await waitFor(() => expect(screen.queryByText("sherpa-onnx-whisper-turbo")).not.toBeInTheDocument());
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(GetUnusedModels).toHaveBeenCalledTimes(2);
});

test("a refused removal shows the message in the dialog and keeps the row", async () => {
  GetUnusedModels.mockResolvedValue(old);
  RemoveUnusedModel.mockRejectedValue("That model is in use and cannot be removed.");
  render(<ModelsCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Remove silero_vad" }));
  await act(async () => {
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }));
  });
  expect(within(screen.getByRole("dialog")).getByRole("alert")).toHaveTextContent("That model is in use and cannot be removed.");
  expect(screen.getByText("silero_vad")).toBeInTheDocument();
});

test("a listing error is shown", async () => {
  GetUnusedModels.mockRejectedValue(new Error("permission denied"));
  render(<ModelsCard />);
  expect(await screen.findByRole("alert")).toHaveTextContent("permission denied");
});
