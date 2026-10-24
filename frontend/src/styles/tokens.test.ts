import fonts from "./fonts.css?raw";
import css from "./tokens.css?raw";
import { blockAfter, parseVars } from "./contrast";

const bundled = Object.keys(import.meta.glob("../assets/fonts/*", { query: "?url", eager: false }));

const light = parseVars(blockAfter(css, ":root {"));

const expected: Record<string, string> = {
  "--bg": "#fbfbf8",
  "--bar": "#f1f2ee",
  "--side": "#f1f2ee",
  "--card": "#ffffff",
  "--fg": "#1d2330",
  "--fg2": "#4b5263",
  "--fg3": "#7c8292",
  "--fg3-text": "#686e7e",
  "--border": "#dfe1da",
  "--sel": "#e3e7f5",
  "--dot": "#d3d5cd",
  "--accent": "#24408e",
  "--on-accent": "#ffffff",
  "--rec": "#b4322a",
  "--rec-bg": "#f6e3e1",
  "--wave": "#9aa6c8",
  "--speaker": "#24408e",
  "--font-sans": '"IBM Plex Sans", sans-serif',
  "--font-display": '"Newsreader", Georgia, serif',
  "--font-notes": '"Newsreader", Georgia, serif',
  "--notes-size": "14.5px",
  "--h3": "19px",
  "--radius": "10px",
  "--r-sm": "5px",
  "--r-md": "6px",
  "--shadow": "0 1px 2px rgba(29,35,48,.06), 0 14px 30px -14px rgba(29,35,48,.2)",
};

test.each(Object.entries(expected))("light %s", (k, v) => {
  expect(light[k]).toBe(v);
});

describe("fonts.css", () => {
  test("fetches nothing from the network", () => {
    expect(fonts).not.toMatch(/https?:/i);
  });
  test("every url() points at a bundled file", () => {
    const urls = [...fonts.matchAll(/url\("([^"]+)"\)/g)].map((m) => m[1]);
    expect(urls.length).toBeGreaterThanOrEqual(8);
    for (const u of urls) expect(bundled, u).toContain(u.replace("../assets/fonts/", "../assets/fonts/"));
  });
  test("ships both OFL notices", () => {
    for (const f of ["OFL-Newsreader.txt", "OFL-IBMPlexSans.txt"]) {
      expect(bundled).toContain(`../assets/fonts/${f}`);
    }
  });
});
