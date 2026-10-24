import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ClientCard } from "./ClientCard";
import { ListClients, SetMeetingClient, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

const renderCard = (over: Partial<{ clientKey: string; hasSession: boolean }> = {}, onChanged = vi.fn()) =>
  render(<ClientCard meetingId="m1" clientKey="" hasSession={false} onChanged={onChanged} {...over} />);

test("lists the clients with a placeholder", async () => {
  renderCard();
  expect(screen.getByText("Client")).toBeInTheDocument();
  const select = await screen.findByLabelText("Client for this meeting");
  await screen.findByRole("option", { name: "acme" });
  expect(screen.getByRole("option", { name: "Choose a client" })).toBeInTheDocument();
  expect(screen.getByRole("option", { name: "globex" })).toBeInTheDocument();
  expect(select).toHaveValue("");
});

test("change calls SetMeetingClient and reports it", async () => {
  const onChanged = vi.fn();
  renderCard({}, onChanged);
  const select = await screen.findByLabelText("Client for this meeting");
  await screen.findByRole("option", { name: "acme" });
  fireEvent.change(select, { target: { value: "acme" } });
  expect(SetMeetingClient).toHaveBeenCalledWith("m1", "acme");
  await waitFor(() => expect(onChanged).toHaveBeenCalled());
  expect(select).toHaveValue("acme");
});

test("a rejection shows its message and the select reverts", async () => {
  SetMeetingClient.mockRejectedValue(new Error("The client file is missing."));
  renderCard({ clientKey: "globex" });
  const select = await screen.findByLabelText("Client for this meeting");
  await screen.findByRole("option", { name: "acme" });
  fireEvent.change(select, { target: { value: "acme" } });
  expect(await screen.findByText("The client file is missing.")).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent("The client file is missing.");
  expect(select).toHaveValue("globex");
});

test("empty list text", async () => {
  ListClients.mockResolvedValue([]);
  renderCard();
  expect(await screen.findByText("No client files found in instructions/clients/ of the archive folder.")).toBeInTheDocument();
});

test("a ListClients rejection shows its text verbatim", async () => {
  ListClients.mockRejectedValue("Choose the archive folder in Settings first.");
  renderCard();
  expect(await screen.findByText("Choose the archive folder in Settings first.")).toBeInTheDocument();
});

test("locked with a note once the meeting has a session", async () => {
  renderCard({ clientKey: "acme", hasSession: true });
  const select = await screen.findByLabelText("Client for this meeting");
  expect(select).toBeDisabled();
  expect(screen.getByText("The client is fixed once the summary exists.")).toBeInTheDocument();
});
