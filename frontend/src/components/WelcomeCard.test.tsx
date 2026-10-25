import { render, screen, within } from "@testing-library/react";
import { WelcomeCard } from "./WelcomeCard";

const STEP1 = "Choose your archive folder, or clone it from GitHub.";
const STEP2 = "Choose your profile, or create a new one.";
const STEP3 = "Add your projects and, if you like, their root folders.";

function items() {
  return screen.getAllByRole("listitem");
}

test("shows the header, the body and the three steps in order", () => {
  render(<WelcomeCard saved={{ profile: "", checkoutPath: "", projectRoots: {} }} />);
  expect(screen.getByText("Set up Notted")).toBeInTheDocument();
  expect(screen.getByText("Notted needs your meetings archive and your profile in it before it can write a summary.")).toBeInTheDocument();
  const li = items();
  expect(li).toHaveLength(3);
  expect(li[0]).toHaveTextContent(STEP1);
  expect(li[1]).toHaveTextContent(STEP2);
  expect(li[2]).toHaveTextContent(STEP3);
  expect(screen.queryByText("Done")).not.toBeInTheDocument();
});

test("Done follows step 1 only when the archive is set", () => {
  render(<WelcomeCard saved={{ profile: "", checkoutPath: "/archive", projectRoots: {} }} />);
  const li = items();
  expect(within(li[0]).getByText("Done")).toBeInTheDocument();
  expect(within(li[1]).queryByText("Done")).not.toBeInTheDocument();
  expect(within(li[2]).queryByText("Done")).not.toBeInTheDocument();
});

test("Done follows step 2 only when the profile is set; step 3 never has a marker", () => {
  render(<WelcomeCard saved={{ profile: "ana", checkoutPath: "", projectRoots: {} }} />);
  const li = items();
  expect(within(li[0]).queryByText("Done")).not.toBeInTheDocument();
  expect(within(li[1]).getByText("Done")).toBeInTheDocument();
  expect(within(li[2]).queryByText("Done")).not.toBeInTheDocument();
});

test("points at the Set up with Claude Code card and adds no step", () => {
  render(<WelcomeCard saved={{ profile: "", checkoutPath: "", projectRoots: {} }} />);
  expect(screen.getByText(/Use Set up with Claude Code below to copy a prompt/)).toBeInTheDocument();
  expect(items()).toHaveLength(3);
});
