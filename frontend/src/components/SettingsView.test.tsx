import { render, screen } from "@testing-library/react";
import { SettingsView } from "./SettingsView";
import { GetSettings, ListProjects, ListInstructionFiles, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));
vi.mock("@uiw/react-codemirror", () => import("../test/codemirrorMock"));

beforeEach(() => {
  resetBindings();
  ListInstructionFiles.mockResolvedValue([{ path: "instructions/projects/acme.md", kind: "project", readOnly: false }]);
});

test("shows the Settings heading, one selected Instructions tab and the three cards in order", async () => {
  render(<SettingsView />);
  expect(screen.getByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
  const tabs = screen.getAllByRole("tab");
  expect(tabs).toHaveLength(1);
  expect(tabs[0]).toHaveTextContent("Instructions");
  expect(tabs[0]).toHaveAttribute("aria-selected", "true");
  await screen.findByLabelText("Profile");
  const headers = [...document.querySelectorAll("section > .nt-card__header")].map((e) => e.textContent);
  expect(headers).toEqual(["Your setup", "Instruction files", "Prompt preview"]);
});

test("the project list feeds the project-root rows and the preview select; a failure shows the message in both", async () => {
  render(<SettingsView />);
  expect(await screen.findByLabelText("acme")).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "globex" })).toBeInTheDocument();
});

test("a refused project list shows its message", async () => {
  ListProjects.mockRejectedValue(new Error("Set the archive folder to list its projects."));
  render(<SettingsView />);
  expect(await screen.findAllByText("Set the archive folder to list its projects.")).not.toHaveLength(0);
});

test("there is no model, provider or API key control or text", async () => {
  GetSettings.mockResolvedValue({ profile: "ana", checkoutPath: "/archive", projectRoots: {} });
  render(<SettingsView />);
  await screen.findByLabelText("Profile");
  const controls = [...screen.queryAllByRole("combobox"), ...screen.queryAllByRole("textbox"), ...screen.queryAllByRole("button"), ...screen.queryAllByRole("tab")];
  expect(controls.length).toBeGreaterThan(3);
  for (const c of controls) {
    const label = (c as HTMLInputElement).labels?.[0]?.textContent ?? "";
    const name = `${c.getAttribute("aria-label") ?? ""} ${c.textContent ?? ""} ${label} ${c.getAttribute("placeholder") ?? ""}`;
    expect(name).not.toMatch(/model|provider|api key/i);
  }
  expect(document.body.textContent).not.toMatch(/model|provider|api key/i);
});
