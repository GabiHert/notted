import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ProjectCard } from "./ProjectCard";
import { ListProjects, SetMeetingProject, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

const renderCard = (over: Partial<{ projectKey: string; hasSession: boolean }> = {}, onChanged = vi.fn()) =>
  render(<ProjectCard meetingId="m1" projectKey="" hasSession={false} onChanged={onChanged} {...over} />);

test("lists the projects with a placeholder", async () => {
  renderCard();
  expect(screen.getByText("Project")).toBeInTheDocument();
  const select = await screen.findByLabelText("Project for this meeting");
  await screen.findByRole("option", { name: "acme" });
  expect(screen.getByRole("option", { name: "Choose a project" })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "globex" })).toBeInTheDocument();
  expect(select).toHaveValue("");
});

test("change calls SetMeetingProject and reports it", async () => {
  const onChanged = vi.fn();
  renderCard({}, onChanged);
  const select = await screen.findByLabelText("Project for this meeting");
  await screen.findByRole("option", { name: "acme" });
  fireEvent.change(select, { target: { value: "acme" } });
  expect(SetMeetingProject).toHaveBeenCalledWith("m1", "acme");
  await waitFor(() => expect(onChanged).toHaveBeenCalled());
  expect(select).toHaveValue("acme");
});

test("a rejection shows its message and the select reverts", async () => {
  SetMeetingProject.mockRejectedValue(new Error("The project file is missing."));
  renderCard({ projectKey: "globex" });
  const select = await screen.findByLabelText("Project for this meeting");
  await screen.findByRole("option", { name: "acme" });
  fireEvent.change(select, { target: { value: "acme" } });
  expect(await screen.findByText("The project file is missing.")).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent("The project file is missing.");
  expect(select).toHaveValue("globex");
});

test("empty list text", async () => {
  ListProjects.mockResolvedValue([]);
  renderCard();
  expect(await screen.findByText("No project files found in instructions/projects/ of the archive folder.")).toBeInTheDocument();
});

test("a ListProjects rejection shows its text verbatim", async () => {
  ListProjects.mockRejectedValue("Choose the archive folder in Settings first.");
  renderCard();
  expect(await screen.findByText("Choose the archive folder in Settings first.")).toBeInTheDocument();
});

test("locked with a note once the meeting has a session", async () => {
  renderCard({ projectKey: "acme", hasSession: true });
  const select = await screen.findByLabelText("Project for this meeting");
  expect(select).toBeDisabled();
  expect(screen.getByText("The project is fixed once the summary exists.")).toBeInTheDocument();
});
