import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { AppearanceCard } from "./AppearanceCard";
import { builtinThemes, DeleteTheme, ExportTheme, GetSelectedTheme, ImportVSCodeTheme, ListThemes, resetBindings, SelectTheme } from "../test/bindingsMock";
import { applyTheme, THEME_EVENT } from "../theme";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

const custom = {
  id: "harbor-night",
  name: "Harbor Night",
  mode: "dark",
  builtin: false,
  tokens: { bg: "#1a1b26", bar: "#16161e", side: "#16161e", card: "#1f2030", fg: "#c0caf5", fg2: "#7d83a2", fg3: "#848bb0", "fg3-text": "#848bb0", border: "#101014", sel: "#2d3650", dot: "#3b3e4f", accent: "#6787cd", "on-accent": "#0a0e15", rec: "#f7768e", "rec-bg": "#3d2a37", wave: "#4c6193", speaker: "#73daca" },
};

const root = () => document.documentElement;

beforeEach(() => {
  resetBindings();
  applyTheme(null);
});

test("lists the built-in themes first, then the custom ones, with the chosen one checked", async () => {
  ListThemes.mockResolvedValue([...builtinThemes(), custom]);
  render(<AppearanceCard />);
  const group = await screen.findByRole("radiogroup", { name: "Theme" });
  await waitFor(() => expect(within(group).getAllByRole("radio")).toHaveLength(4));
  const names = within(group).getAllByRole("radio").map((r) => (r as HTMLInputElement).labels?.[0]?.textContent);
  expect(names).toEqual(["Oscilloscope (match system)", "Oscilloscope Dark", "Oscilloscope Light", "Harbor NightCustom, dark"]);
  expect(screen.getByRole("radio", { name: "Oscilloscope (match system)" })).toBeChecked();
  // Export and Delete are offered on custom themes only.
  expect(screen.getAllByRole("button", { name: /^Export / })).toHaveLength(1);
  expect(screen.getByRole("button", { name: "Delete Harbor Night" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Delete Oscilloscope Dark" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Import VS Code theme…" })).toBeInTheDocument();
});

test("picking a custom theme calls the binding and sets data-theme and every token on <html>", async () => {
  ListThemes.mockResolvedValue([...builtinThemes(), custom]);
  SelectTheme.mockResolvedValue(custom);
  const heard = vi.fn();
  window.addEventListener(THEME_EVENT, heard);
  render(<AppearanceCard />);
  fireEvent.click(await screen.findByRole("radio", { name: /Harbor Night/ }));
  await waitFor(() => expect(SelectTheme).toHaveBeenCalledWith("harbor-night"));
  await waitFor(() => expect(root()).toHaveAttribute("data-theme", "dark"));
  for (const [k, v] of Object.entries(custom.tokens)) expect(root().style.getPropertyValue(`--${k}`)).toBe(v);
  expect(screen.getByRole("radio", { name: /Harbor Night/ })).toBeChecked();
  expect(heard).toHaveBeenCalled();
  window.removeEventListener(THEME_EVENT, heard);
});

test("a built-in theme sets data-theme only, and system clears it and the inline tokens", async () => {
  applyTheme(custom);
  render(<AppearanceCard />);
  // Loading the chosen theme (system) gives tokens.css back to macOS.
  await waitFor(() => expect(root()).not.toHaveAttribute("data-theme"));
  expect(root().style.getPropertyValue("--bg")).toBe("");

  fireEvent.click(await screen.findByRole("radio", { name: "Oscilloscope Light" }));
  await waitFor(() => expect(root()).toHaveAttribute("data-theme", "light"));
  expect(SelectTheme).toHaveBeenCalledWith("oscilloscope-light");
  expect(root().style.getPropertyValue("--bg")).toBe("");

  fireEvent.click(screen.getByRole("radio", { name: "Oscilloscope (match system)" }));
  await waitFor(() => expect(root()).not.toHaveAttribute("data-theme"));
});

test("a failed import shows the binding's error and changes nothing", async () => {
  ImportVSCodeTheme.mockRejectedValue(new Error('This file is not a VS Code colour theme: it needs a "colors" object with keys such as editor.background and editor.foreground.'));
  render(<AppearanceCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Import VS Code theme…" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("This file is not a VS Code colour theme");
  expect(SelectTheme).not.toHaveBeenCalled();
});

test("an import adds the theme and switches to it; a cancelled one does nothing", async () => {
  render(<AppearanceCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Import VS Code theme…" }));
  await waitFor(() => expect(ImportVSCodeTheme).toHaveBeenCalledTimes(1));
  expect(SelectTheme).not.toHaveBeenCalled();

  ImportVSCodeTheme.mockResolvedValue(custom);
  SelectTheme.mockResolvedValue(custom);
  ListThemes.mockResolvedValue([...builtinThemes(), custom]);
  GetSelectedTheme.mockResolvedValue(custom);
  fireEvent.click(screen.getByRole("button", { name: "Import VS Code theme…" }));
  await waitFor(() => expect(SelectTheme).toHaveBeenCalledWith("harbor-night"));
  expect(await screen.findByRole("radio", { name: /Harbor Night/ })).toBeChecked();
  expect(screen.getByRole("status")).toHaveTextContent("Imported Harbor Night.");
  expect(root().style.getPropertyValue("--accent")).toBe("#6787cd");
});

test("delete asks in the confirm dialog first, and cancel keeps the theme", async () => {
  ListThemes.mockResolvedValue([...builtinThemes(), custom]);
  const confirmSpy = vi.spyOn(window, "confirm");
  render(<AppearanceCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Delete Harbor Night" }));
  const dialog = screen.getByRole("dialog", { name: "Delete theme?" });
  expect(dialog).toHaveTextContent("Delete the theme Harbor Night?");
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(DeleteTheme).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "Delete Harbor Night" }));
  ListThemes.mockResolvedValue(builtinThemes());
  await act(async () => {
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete theme" }));
  });
  expect(DeleteTheme).toHaveBeenCalledWith("harbor-night");
  await waitFor(() => expect(screen.queryByRole("radio", { name: /Harbor Night/ })).not.toBeInTheDocument());
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(confirmSpy).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});

test("a failed delete keeps the dialog open with the error", async () => {
  ListThemes.mockResolvedValue([...builtinThemes(), custom]);
  DeleteTheme.mockRejectedValue(new Error("permission denied"));
  render(<AppearanceCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Delete Harbor Night" }));
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Delete theme" }));
  expect(await within(screen.getByRole("dialog")).findByRole("alert")).toHaveTextContent("permission denied");
});

test("export reports where the file went", async () => {
  ListThemes.mockResolvedValue([...builtinThemes(), custom]);
  ExportTheme.mockResolvedValue("/Users/ana/Downloads/harbor-night.phonon-theme.json");
  render(<AppearanceCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Export Harbor Night" }));
  expect(await screen.findByRole("status")).toHaveTextContent("Exported Harbor Night to /Users/ana/Downloads/harbor-night.phonon-theme.json.");
  expect(ExportTheme).toHaveBeenCalledWith("harbor-night");
});
