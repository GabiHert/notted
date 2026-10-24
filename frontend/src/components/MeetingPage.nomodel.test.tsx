import { render, screen } from "@testing-library/react";
import { MeetingPage } from "./MeetingPage";
import { GetMeeting, GetModelStatus, meeting, resetBindings } from "../test/bindingsMock";

vi.mock("../api/bindings", () => import("../test/bindingsMock"));

beforeEach(resetBindings);

test("a summarized meeting page has no model, provider or API key control", async () => {
  GetModelStatus.mockResolvedValue({ language: "en", ready: true, missing: "", downloadBytes: 0, downloading: false });
  GetMeeting.mockResolvedValue(meeting({ state: "summarized", summaryMd: "# Summary: Weekly sync", suggestionsMd: "# Suggestions" }));
  render(<MeetingPage meetingId="m1" />);
  await screen.findByRole("heading", { name: "Summary: Weekly sync" });
  const controls = [...screen.queryAllByRole("combobox"), ...screen.queryAllByRole("textbox"), ...screen.queryAllByRole("button")];
  expect(controls.length).toBeGreaterThan(0);
  for (const c of controls) {
    const name = c.getAttribute("aria-label") ?? c.textContent ?? "";
    expect(name).not.toMatch(/model|provider|api key/i);
  }
  for (const label of screen.queryAllByRole("combobox")) {
    expect(label).toHaveAccessibleName("Client for this meeting");
  }
  expect(document.body.textContent).not.toMatch(/provider|api key/i);
});

test("the Client, Summary, Suggestions and session cards sit in that order below the transcript", async () => {
  GetMeeting.mockResolvedValue(meeting());
  render(<MeetingPage meetingId="m1" />);
  await screen.findByText("Transcript");
  const headers = [...document.querySelectorAll(".nt-card__header")].map((e) => e.textContent);
  expect(headers).toEqual(["Transcript", "Client", "Summary", "Suggestions", "Claude Code session"]);
});
