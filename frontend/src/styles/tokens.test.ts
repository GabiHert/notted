import fonts from "./fonts.css?raw";
import css from "./tokens.css?raw";
import { blockAfter, parseVars } from "./contrast";

const bundled = Object.keys(import.meta.glob("../assets/fonts/*", { query: "?url", eager: false }));

const light = parseVars(blockAfter(css, ":root {"));

const expected: Record<string, string> = {
  "--bg": "#f4f3f0",
  "--bar": "#ebe9e4",
  "--side": "#ebe9e4",
  "--card": "#fbfaf8",
  "--fg": "#1b1a17",
  "--fg2": "#4f4b43",
  "--fg3": "#736e64",
  "--fg3-text": "#6a655b",
  "--border": "#dcd8d0",
  "--sel": "#f3e3c6",
  "--dot": "#cdc8be",
  "--accent": "#9a5600",
  "--on-accent": "#ffffff",
  "--rec": "#b8321f",
  "--rec-bg": "#f5e0d9",
  "--wave": "#d8a75e",
  "--speaker": "#9a5600",
  "--font-sans": '"Geist", -apple-system, system-ui, sans-serif',
  "--font-display": '"Geist", -apple-system, system-ui, sans-serif',
  "--font-notes": '"Geist", -apple-system, system-ui, sans-serif',
  "--font-mono": '"Geist Mono", ui-monospace, Menlo, monospace',
  "--notes-size": "14px",
  "--h3": "17px",
  "--radius": "7px",
  "--r-sm": "4px",
  "--r-md": "5px",
  "--shadow": "0 1px 2px rgba(27,26,23,.06), 0 14px 30px -14px rgba(27,26,23,.22)",
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
    expect(urls.length).toBeGreaterThanOrEqual(2);
    for (const u of urls) expect(bundled, u).toContain(u.replace("../assets/fonts/", "../assets/fonts/"));
  });
  test("ships the Geist OFL notice", () => {
    expect(bundled).toContain("../assets/fonts/OFL-Geist.txt");
  });
});
