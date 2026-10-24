import { render, screen } from "@testing-library/react";
import { App } from "./App";
import { GetMeeting, ListMeetings, meeting, resetBindings } from "./test/bindingsMock";

vi.mock("./api/bindings", () => import("./test/bindingsMock"));

beforeEach(resetBindings);

test("shows the title bar, the sidebar and the new-recording view when there are no meetings", async () => {
  render(<App />);
  expect(screen.getByText("Notted")).toBeInTheDocument();
  expect(await screen.findByRole("heading", { level: 1, name: "New recording" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
});

test("opens the newest meeting on launch", async () => {
  ListMeetings.mockResolvedValue([meeting({ id: "m2", title: "Newest" }), meeting({ id: "m1", title: "Older" })]);
  GetMeeting.mockImplementation(async (id: string) => meeting({ id, title: id === "m2" ? "Newest" : "Older" }));
  render(<App />);
  expect(await screen.findByRole("heading", { level: 1, name: "Newest" })).toBeInTheDocument();
});
