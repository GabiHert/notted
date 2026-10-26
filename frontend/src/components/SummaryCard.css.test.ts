import css from "./SummaryCard.css?raw";

const body = (selector: string) => {
  const m = css.match(new RegExp(selector.replace(/[.\s]/g, (c) => (c === "." ? "\\." : "\\s+")) + "\\s*\\{([^}]*)\\}"));
  return m ? m[1] : "";
};

test(".nt-review__reply is as wide as the card and wraps long words", () => {
  const b = body(".nt-review__reply");
  expect(b).toContain("align-self: stretch");
  expect(b).toContain("min-width: 0");
  expect(b).toContain("max-width: 100%");
  expect(b).toContain("overflow-wrap: anywhere");
});

test("the reply's code blocks wrap instead of scrolling sideways", () => {
  const b = body(".nt-review__reply .nt-md pre");
  expect(b).toContain("white-space: pre-wrap");
  expect(b).toContain("overflow-wrap: anywhere");
});
