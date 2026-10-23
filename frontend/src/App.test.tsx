import { render, screen } from "@testing-library/react";
import { App } from "./App";

test("shows the Notted heading", () => {
  render(<App />);
  expect(screen.getByRole("heading", { name: "Notted" })).toBeInTheDocument();
});
