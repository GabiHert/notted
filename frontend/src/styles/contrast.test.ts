import tokensCss from "./tokens.css?raw";
import { blockAfter, parseVars, ratio } from "./contrast";

const css = tokensCss;
const light = parseVars(blockAfter(css, ":root {"));
const mediaStart = css.indexOf("@media (prefers-color-scheme: dark)");
const dark = mediaStart < 0 ? {} : parseVars(blockAfter(css, ':root:not([data-theme="light"])', mediaStart));
const darkOverride = mediaStart < 0 ? {} : parseVars(blockAfter(css, '[data-theme="dark"]', css.indexOf("}", mediaStart)));

const darkPairs: [string, string][] = [
  ["--fg", "--bg"],
  ["--fg2", "--bg"],
  ["--fg3", "--bg"],
  ["--fg", "--card"],
  ["--fg3", "--card"],
  ["--fg3", "--side"],
  ["--on-accent", "--accent"],
  ["--rec", "--rec-bg"],
  ["--speaker", "--bg"],
  ["--accent", "--sel"],
];
const lightPairs: [string, string][] = [
  ["--fg3-text", "--bg"],
  ["--fg3-text", "--side"],
  ["--fg3-text", "--card"],
];
const lightReportOnly: [string, string][] = [
  ["--fg", "--bg"],
  ["--fg2", "--bg"],
  ["--fg3", "--bg"],
  ["--fg3", "--card"],
  ["--fg3", "--side"],
  ["--on-accent", "--accent"],
  ["--rec", "--rec-bg"],
  ["--speaker", "--bg"],
  ["--accent", "--sel"],
];

describe("dark tokens", () => {
  test("the media block and the data-theme override carry the same values", () => {
    expect(Object.keys(dark).length).toBeGreaterThan(15);
    expect(darkOverride).toEqual(dark);
  });
  test("every colour token has a dark value", () => {
    for (const k of Object.keys(light).filter((k) => /^#/.test(light[k]))) {
      expect(dark, k).toHaveProperty(k);
    }
  });
  test.each(darkPairs)("dark %s on %s reaches AA (4.5:1)", (fg, bg) => {
    expect(ratio(dark[fg], dark[bg])).toBeGreaterThanOrEqual(4.5);
  });
  test("dark --fg3-text reaches AA on bg, side and card", () => {
    for (const bg of ["--bg", "--side", "--card"]) {
      expect(ratio(dark["--fg3-text"], dark[bg])).toBeGreaterThanOrEqual(4.5);
    }
  });
  test("dark surfaces are not pure black", () => {
    for (const k of ["--bg", "--bar", "--side", "--card"]) expect(dark[k].toLowerCase()).not.toBe("#000000");
  });
});

describe("light tokens", () => {
  test.each(lightPairs)("light %s on %s reaches AA (4.5:1)", (fg, bg) => {
    expect(ratio(light[fg], light[bg])).toBeGreaterThanOrEqual(4.5);
  });
  test("reports the remaining light ratios without failing", () => {
    const rows = lightReportOnly.map(([a, b]) => `${a} on ${b}: ${ratio(light[a], light[b]).toFixed(2)}`);
    console.info(`light ratios\n${rows.join("\n")}`);
    expect(rows.length).toBe(lightReportOnly.length);
  });
});
