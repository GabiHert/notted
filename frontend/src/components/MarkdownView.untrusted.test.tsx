import { render } from "@testing-library/react";
import { MarkdownView } from "./MarkdownView";

const link = "[the guide](" + "https:" + "//example.invalid/guide)";
const img = "![a chart](" + "https:" + "//example.invalid/p.png)";

test("untrusted: a link is a span with the address in title, no anchor", () => {
  const { container } = render(<MarkdownView content={link} untrusted />);
  expect(container.querySelector("a")).toBeNull();
  const span = container.querySelector("span.nt-md__link")!;
  expect(span.textContent).toBe("the guide");
  expect(span.getAttribute("title")).toBe("https:" + "//example.invalid/guide");
});

test("untrusted: an image is its alt text, no img", () => {
  const { container } = render(<MarkdownView content={img} untrusted />);
  expect(container.querySelector("img")).toBeNull();
  expect(container.textContent).toContain("a chart");
});

test("untrusted: raw HTML is not rendered as an element", () => {
  const { container } = render(<MarkdownView content={"before <b>bold</b> after"} untrusted />);
  expect(container.querySelector("b")).toBeNull();
});

test("trusted default still renders links", () => {
  const { container } = render(<MarkdownView content={link} />);
  expect(container.querySelector("a")).not.toBeNull();
});
