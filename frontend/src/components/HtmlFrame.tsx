// Draws a session's HTML in an inert frame: sandbox with no allow-* token, and a policy that
// allows only inline style and data: images. The fragment is inserted as is.
export function frameDocument(html: string, scheme: "light" | "dark", bg: string, fg: string): string {
  return (
    `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:">` +
    `<style>:root{color-scheme:${scheme}}body{margin:12px;font:14px/1.5 -apple-system,BlinkMacSystemFont,sans-serif;background:${bg};color:${fg}}</style></head><body>${html}</body></html>`
  );
}

export function HtmlFrame({ html }: { html: string }) {
  const theme = document.documentElement.getAttribute("data-theme");
  const scheme: "light" | "dark" =
    theme === "light" || theme === "dark" ? theme : window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  const style = getComputedStyle(document.documentElement);
  const bg = style.getPropertyValue("--card").trim() || "#ffffff";
  const fg = style.getPropertyValue("--fg").trim() || "#1d2330";
  return <iframe className="nt-form__frame" sandbox="" srcDoc={frameDocument(html, scheme, bg, fg)} title="Content from Claude Code" referrerPolicy="no-referrer" />;
}
