import { act, fireEvent, render, screen } from "@testing-library/react";
import { Sidebar } from "./Sidebar";
import { ListMeetings, emit, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

const two = () => [
  meeting({ id: "m2", title: "Planning review", language: "pt-BR", state: "recording", startedAt: "2026-10-23T16:00:00" }),
  meeting({ id: "m1", title: "Weekly sync" }),
];

test("renders a row per meeting with title, meta line and state dot", async () => {
  ListMeetings.mockResolvedValue(two());
  const { container } = render(<Sidebar view={{ kind: "new" }} onSelect={() => {}} />);
  expect(await screen.findByText("Weekly sync")).toBeInTheDocument();
  expect(screen.getByText("Oct 23, 2026, 14:05 · English · Ready")).toBeInTheDocument();
  expect(screen.getByText("Oct 23, 2026, 16:00 · Portuguese (Brazil) · Recording")).toBeInTheDocument();
  const dots = container.querySelectorAll(".nt-dot");
  expect(dots).toHaveLength(2);
  expect(dots[0]).toHaveClass("nt-dot--rec");
  expect(dots[1]).not.toHaveClass("nt-dot--rec");
});

test("marks the selected row", async () => {
  ListMeetings.mockResolvedValue(two());
  render(<Sidebar view={{ kind: "meeting", id: "m1" }} onSelect={() => {}} />);
  const row = (await screen.findByText("Weekly sync")).closest("button")!;
  expect(row).toHaveAttribute("aria-current", "true");
  expect(screen.getByText("Planning review").closest("button")).not.toHaveAttribute("aria-current");
});

test("shows the empty text", async () => {
  render(<Sidebar view={{ kind: "new" }} onSelect={() => {}} />);
  expect(await screen.findByText("No meetings yet.")).toBeInTheDocument();
});

test("New recording, a row and Settings switch the view", async () => {
  ListMeetings.mockResolvedValue(two());
  const onSelect = vi.fn();
  render(<Sidebar view={{ kind: "new" }} onSelect={onSelect} />);
  fireEvent.click(screen.getByRole("button", { name: "New recording" }));
  expect(onSelect).toHaveBeenLastCalledWith({ kind: "new" });
  fireEvent.click(screen.getByRole("button", { name: "Settings" }));
  expect(onSelect).toHaveBeenLastCalledWith({ kind: "settings" });
  fireEvent.click(await screen.findByText("Planning review"));
  expect(onSelect).toHaveBeenLastCalledWith({ kind: "meeting", id: "m2" });
});

test("refreshes the list on recording:state", async () => {
  ListMeetings.mockResolvedValueOnce([]);
  render(<Sidebar view={{ kind: "new" }} onSelect={() => {}} />);
  await screen.findByText("No meetings yet.");
  ListMeetings.mockResolvedValueOnce(two());
  await act(async () => emit("recording:state", { meetingId: "m2", state: "recording" }));
  expect(await screen.findByText("Planning review")).toBeInTheDocument();
  expect(ListMeetings).toHaveBeenCalledTimes(2);
});
