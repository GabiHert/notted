import { render } from "@testing-library/react";
import { HtmlFrame, frameDocument } from "./HtmlFrame";

const HEAD = `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:">`;

afterEach(() => document.documentElement.removeAttribute("data-theme"));

test("the frame is sandboxed with no allow tokens and has a title", () => {
  const { container } = render(<HtmlFrame html="<p>Hi</p>" />);
  const frame = container.querySelector("iframe")!;
  expect(frame.getAttribute("sandbox")).toBe("");
  expect(frame.getAttribute("title")).toBe("Content from Claude Code");
  expect(frame.getAttribute("referrerpolicy")).toBe("no-referrer");
});

test("srcdoc starts with the exact policy and holds the fragment in the body", () => {
  const { container } = render(<HtmlFrame html="<p>Hi</p>" />);
  const doc = container.querySelector("iframe")!.getAttribute("srcdoc")!;
  expect(doc.startsWith(HEAD)).toBe(true);
  expect(doc).toContain("<body><p>Hi</p></body>");
});

test("data-theme dark gives color-scheme dark", () => {
  document.documentElement.setAttribute("data-theme", "dark");
  const { container } = render(<HtmlFrame html="x" />);
  expect(container.querySelector("iframe")!.getAttribute("srcdoc")).toContain("color-scheme:dark");
});

test("data-theme light gives color-scheme light", () => {
  document.documentElement.setAttribute("data-theme", "light");
  const { container } = render(<HtmlFrame html="x" />);
  expect(container.querySelector("iframe")!.getAttribute("srcdoc")).toContain("color-scheme:light");
});

test("frameDocument returns the exact document", () => {
  expect(frameDocument("<i>a</i>", "dark", "#111111", "#eeeeee")).toBe(
    HEAD +
      "<style>:root{color-scheme:dark}body{margin:12px;font:14px/1.5 -apple-system,BlinkMacSystemFont,sans-serif;background:#111111;color:#eeeeee}</style></head><body><i>a</i></body></html>",
  );
});
