// Colour themes. tokens.css paints the app and follows macOS; a theme overrides it on <html>:
// - system: no data-theme and no inline tokens, so tokens.css follows macOS;
// - a built-in light or dark theme: data-theme only, since tokens.css holds its colours;
// - a custom theme: data-theme set to its mode and every token as an inline --var.
import { GetSelectedTheme, type wails } from "./api/bindings";

export type Theme = wails.ThemeDTO;

/** Fired on window after a theme is applied, so canvases such as the terminal repaint. */
export const THEME_EVENT = "phonon:theme";

/** Every colour token a theme sets, without the leading --. */
export const THEME_TOKENS = ["bg", "bar", "side", "card", "fg", "fg2", "fg3", "fg3-text", "border", "sel", "dot", "accent", "on-accent", "rec", "rec-bg", "wave", "speaker"];

const HEX = /^#[0-9a-fA-F]{6}$/;

/** Applies a theme to the document; null or the system theme gives tokens.css back to macOS. */
export function applyTheme(t: Pick<Theme, "id" | "mode" | "builtin" | "tokens"> | null) {
  const el = document.documentElement;
  for (const k of THEME_TOKENS) el.style.removeProperty(`--${k}`);
  if (!t || (t.mode !== "light" && t.mode !== "dark")) {
    el.removeAttribute("data-theme");
  } else {
    el.setAttribute("data-theme", t.mode);
    if (!t.builtin) {
      for (const k of THEME_TOKENS) {
        const v = t.tokens?.[k];
        if (v && HEX.test(v)) el.style.setProperty(`--${k}`, v);
      }
    }
  }
  window.dispatchEvent(new Event(THEME_EVENT));
}

/** Loads the chosen theme from the backend and applies it. A failure leaves tokens.css as it is. */
export async function loadSelectedTheme(): Promise<Theme | null> {
  try {
    const t = await GetSelectedTheme();
    applyTheme(t);
    return t;
  } catch {
    return null;
  }
}
