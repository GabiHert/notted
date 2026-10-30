import { act, fireEvent, render, screen } from "@testing-library/react";
import { ModelBanner } from "./ModelBanner";
import { DownloadModels, GetModelStatus, emit, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

const missing = (over = {}) => ({ language: "pt-BR", ready: false, missing: "whisper", downloadBytes: 563_800_000, downloading: false, ...over });
const progress = (over = {}) => ({ language: "pt-BR", name: "whisper-turbo", phase: "download", bytesDone: 50, bytesTotal: 200, ...over });

test("not ready: shows the size rounded to MB and the download button", async () => {
  GetModelStatus.mockResolvedValue(missing());
  render(<ModelBanner language="pt-BR" />);
  expect(await screen.findByText("Speech models")).toBeInTheDocument();
  expect(
    screen.getByText(
      "The speech models for Portuguese (Brazil) are not on this Mac yet (about 564 MB). Recording works now; transcription starts once they are downloaded.",
    ),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Download models" }));
  expect(DownloadModels).toHaveBeenCalledWith("pt-BR");
});

test("download, verify and extract phases", async () => {
  GetModelStatus.mockResolvedValue(missing());
  render(<ModelBanner language="pt-BR" />);
  await screen.findByText("Speech models");
  await act(async () => emit("models:progress", progress()));
  expect(screen.getByText("Downloading whisper-turbo… 25%")).toBeInTheDocument();
  await act(async () => emit("models:progress", progress({ phase: "verify" })));
  expect(screen.getByText("Checking whisper-turbo…")).toBeInTheDocument();
  await act(async () => emit("models:progress", progress({ phase: "extract" })));
  expect(screen.getByText("Unpacking whisper-turbo…")).toBeInTheDocument();
});

test("ignores progress for another language", async () => {
  GetModelStatus.mockResolvedValue(missing());
  render(<ModelBanner language="pt-BR" />);
  await screen.findByText("Speech models");
  await act(async () => emit("models:progress", progress({ language: "en" })));
  expect(screen.queryByText(/Downloading/)).not.toBeInTheDocument();
});

test("error shows the message and Try again downloads again", async () => {
  GetModelStatus.mockResolvedValue(missing());
  render(<ModelBanner language="pt-BR" />);
  await screen.findByText("Speech models");
  await act(async () => emit("models:progress", progress({ phase: "error", error: "network down" })));
  expect(screen.getByText("The download failed: network down")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  expect(DownloadModels).toHaveBeenCalledWith("pt-BR");
});

test("hidden when ready", async () => {
  GetModelStatus.mockResolvedValue(missing({ ready: true }));
  const { container } = render(<ModelBanner language="pt-BR" />);
  await act(async () => {});
  expect(container).toBeEmptyDOMElement();
});

test("hidden after done", async () => {
  GetModelStatus.mockResolvedValue(missing());
  render(<ModelBanner language="pt-BR" />);
  await screen.findByText("Speech models");
  await act(async () => emit("models:progress", progress({ name: "", phase: "done" })));
  expect(screen.queryByText("Speech models")).not.toBeInTheDocument();
});
