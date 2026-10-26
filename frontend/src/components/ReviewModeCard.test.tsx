import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ReviewModeCard } from "./ReviewModeCard";
import { GetReviewActWithoutAsking, SetReviewActWithoutAsking, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

const LABEL = "Let Claude act without asking during reviews";
const HELP = "When this is on, Claude edits files and runs commands, git included, during Review suggestions without asking you first. You still answer the suggestions form. The Claude Code session on a meeting's page still asks in the terminal. A change applies from the next review.";
const box = () => screen.getByLabelText(LABEL) as HTMLInputElement;

beforeEach(() => {
  resetBindings();
});

test("renders the header, the label and the helper with the exact text, and ties the label to the box", async () => {
  render(<ReviewModeCard />);
  expect(screen.getByText("Suggestion reviews", { selector: ".nt-card__header" })).toBeInTheDocument();
  expect(screen.getByText(HELP)).toBeInTheDocument();
  expect(box()).toHaveAttribute("type", "checkbox");
  expect(box()).toHaveAttribute("aria-describedby", screen.getByText(HELP).id);
  await waitFor(() => expect(box()).not.toBeDisabled());
});

test("the box is disabled until the value loads, then reflects it", async () => {
  let resolve!: (v: boolean) => void;
  GetReviewActWithoutAsking.mockReturnValue(new Promise<boolean>((r) => (resolve = r)));
  render(<ReviewModeCard />);
  expect(box()).toBeDisabled();
  await act(async () => resolve(true));
  expect(box()).not.toBeDisabled();
  expect(box()).toBeChecked();
});

test("a stored false loads unchecked", async () => {
  render(<ReviewModeCard />);
  await waitFor(() => expect(box()).not.toBeDisabled());
  expect(box()).not.toBeChecked();
});

test("a click saves true and shows the box checked", async () => {
  render(<ReviewModeCard />);
  await waitFor(() => expect(box()).not.toBeDisabled());
  fireEvent.click(box());
  await waitFor(() => expect(box()).toBeChecked());
  expect(SetReviewActWithoutAsking).toHaveBeenCalledWith(true);
  fireEvent.click(box());
  await waitFor(() => expect(box()).not.toBeChecked());
  expect(SetReviewActWithoutAsking).toHaveBeenLastCalledWith(false);
});

test("the box is disabled while the save is pending, and a second click is ignored", async () => {
  let resolve!: () => void;
  SetReviewActWithoutAsking.mockReturnValue(new Promise<void>((r) => (resolve = r)));
  render(<ReviewModeCard />);
  await waitFor(() => expect(box()).not.toBeDisabled());
  fireEvent.click(box());
  await waitFor(() => expect(box()).toBeDisabled());
  fireEvent.click(box());
  expect(SetReviewActWithoutAsking).toHaveBeenCalledTimes(1);
  expect(box()).not.toBeChecked();
  await act(async () => resolve());
  expect(box()).toBeChecked();
  expect(box()).not.toBeDisabled();
});

test("a rejected save shows the message in an alert and the box returns to unchecked", async () => {
  SetReviewActWithoutAsking.mockRejectedValue(new Error("Notted could not save the review setting: database is locked"));
  render(<ReviewModeCard />);
  await waitFor(() => expect(box()).not.toBeDisabled());
  fireEvent.click(box());
  expect(await screen.findByRole("alert")).toHaveTextContent("Notted could not save the review setting: database is locked");
  expect(box()).not.toBeChecked();
  expect(box()).not.toBeDisabled();
});

test("a rejected load shows the alert, and the box stays disabled and unchecked", async () => {
  GetReviewActWithoutAsking.mockRejectedValue(new Error("Notted could not read the review setting: boom"));
  render(<ReviewModeCard />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Notted could not read the review setting: boom");
  expect(box()).toBeDisabled();
  expect(box()).not.toBeChecked();
});
