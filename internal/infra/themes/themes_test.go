package themes

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pairs are the token pairs the app draws text or the accent with, and the contrast each needs.
var pairs = []struct {
	fg, bg string
	min    float64
}{
	{"fg", "bg", 4.5}, {"fg2", "bg", 4.5}, {"fg3", "bg", 4.5}, {"fg", "card", 4.5}, {"fg3", "card", 4.5}, {"fg3", "side", 4.5},
	{"fg3-text", "bg", 4.5}, {"fg3-text", "side", 4.5}, {"fg3-text", "card", 4.5},
	{"on-accent", "accent", 3}, {"rec", "rec-bg", 4.5}, {"speaker", "bg", 4.5}, {"accent", "sel", 3},
}

func checkReadable(t *testing.T, th Theme) {
	t.Helper()
	for _, p := range pairs {
		a, err := parseHex(th.Tokens[p.fg])
		if err != nil {
			t.Fatalf("%s: %v", p.fg, err)
		}
		b, err := parseHex(th.Tokens[p.bg])
		if err != nil {
			t.Fatalf("%s: %v", p.bg, err)
		}
		if r := contrast(a, b); r < p.min {
			t.Errorf("%s: %s %s on %s %s has contrast %.2f, want at least %.1f", th.Name, p.fg, th.Tokens[p.fg], p.bg, th.Tokens[p.bg], r, p.min)
		}
	}
}

func checkComplete(t *testing.T, th Theme) {
	t.Helper()
	if len(th.Tokens) != len(TokenNames) {
		t.Errorf("%s has %d tokens, want %d", th.Name, len(th.Tokens), len(TokenNames))
	}
	hex := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	for _, k := range TokenNames {
		if !hex.MatchString(th.Tokens[k]) {
			t.Errorf("%s: token %s = %q, want #rrggbb", th.Name, k, th.Tokens[k])
		}
	}
}

func TestBuiltins(t *testing.T) {
	if Builtins[0].ID != SystemID || Builtins[0].Name != "Oscilloscope (match system)" {
		t.Fatalf("the default must come first: %+v", Builtins[0])
	}
	for _, th := range []Theme{OscilloscopeDark, OscilloscopeLight} {
		checkComplete(t, th)
		if th.Mode == "dark" {
			checkReadable(t, th) // tokens.css enforces the full set only in dark
		}
	}
}

// The two Oscilloscope themes must match tokens.css, which paints the app when no theme is set.
func TestOscilloscopeMatchesTokensCSS(t *testing.T) {
	raw, err := os.ReadFile("../../../frontend/src/styles/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	decl := regexp.MustCompile(`--([a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{3,8})\s*;`)
	block := func(start string, from int) map[string]string {
		i := strings.Index(css[from:], start)
		if i < 0 {
			t.Fatalf("no %q block in tokens.css", start)
		}
		body := css[from+i:]
		body = body[:strings.Index(body, "}")]
		m := map[string]string{}
		for _, x := range decl.FindAllStringSubmatch(body, -1) {
			m[x[1]] = strings.ToLower(x[2])
		}
		return m
	}
	media := strings.Index(css, "@media (prefers-color-scheme: dark)")
	if media < 0 {
		t.Fatal("no dark media block in tokens.css")
	}
	light := block(":root {", 0)
	darkMedia := block(`:root:not([data-theme="light"])`, media)
	darkAttr := block(`[data-theme="dark"] {`, media)
	for _, k := range TokenNames {
		if light[k] != OscilloscopeLight.Tokens[k] {
			t.Errorf("oscilloscope-light %s = %s, tokens.css :root has %q", k, OscilloscopeLight.Tokens[k], light[k])
		}
		if darkMedia[k] != OscilloscopeDark.Tokens[k] {
			t.Errorf("oscilloscope-dark %s = %s, tokens.css dark media block has %q", k, OscilloscopeDark.Tokens[k], darkMedia[k])
		}
		if darkAttr[k] != OscilloscopeDark.Tokens[k] {
			t.Errorf("oscilloscope-dark %s = %s, tokens.css [data-theme=\"dark\"] has %q", k, OscilloscopeDark.Tokens[k], darkAttr[k])
		}
	}
}

func TestStripJSONC(t *testing.T) {
	in := `{
  // a comment
  "a": "two//slashes", /* block
  comment */ "b": [1, 2,],
  "c": "quote \" // not a comment",
  "d": { "e": 1, },
}`
	got := StripJSONC([]byte(in))
	var v map[string]any
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatalf("not JSON after stripping: %v\n%s", err, got)
	}
	if v["a"] != "two//slashes" || v["c"] != `quote " // not a comment` {
		t.Errorf("strings changed: %v", v)
	}
	if b, _ := v["b"].([]any); len(b) != 2 {
		t.Errorf("b = %v", v["b"])
	}
	if strings.Contains(string(got), "comment */") || strings.Contains(string(got), "// a comment") {
		t.Errorf("comments left in %s", got)
	}
}

// A realistic dark theme: 8-digit rgba colours, no button colours, and a description colour too
// dim to read.
const darkVSCode = `{
	// Night Owl-ish, trimmed
	"$schema": "vscode://schemas/color-theme",
	"name": "Acme Midnight",
	"type": "dark",
	"colors": {
		"editor.background": "#1a1b26",
		"editor.foreground": "#c0caf5",
		"foreground": "#a9b1d6",
		"titleBar.activeBackground": "#16161e",
		"sideBar.background": "#16161e",
		"activityBar.background": "#13131a",
		"editorWidget.background": "#1f2030",
		"descriptionForeground": "#2a2c3d", /* far too dim on #1a1b26 */
		"editorLineNumber.foreground": "#3b3f5c",
		"panel.border": "#101014",
		"list.activeSelectionBackground": "#7aa2f733",
		"editor.selectionBackground": "#515c7e4d",
		"focusBorder": "#7aa2f7cc",
		"textLink.foreground": "#73daca",
		"errorForeground": "#f7768e",
		"terminal.ansiRed": "#f7768e",
	},
	"tokenColors": [],
}`

const lightVSCode = `{
	"name": "Paper Light",
	"type": "light",
	"colors": {
		"editor.background": "#fafafa",
		"editor.foreground": "#383a42",
		"sideBar.background": "#f0f0f1",
		"dropdown.background": "#ffffff",
		"descriptionForeground": "#c8c8c8",
		"disabledForeground": "#a0a1a7",
		"editorGroup.border": "#e5e5e6",
		"editor.selectionBackground": "#526fff26",
		"button.background": "#4078f2",
		"button.foreground": "#ffffff",
		"editorError.foreground": "#e45649",
	}
}`

func TestFromVSCodeDark(t *testing.T) {
	th, err := FromVSCode([]byte(darkVSCode), "")
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "Acme Midnight" || th.Mode != "dark" {
		t.Fatalf("got name %q mode %q", th.Name, th.Mode)
	}
	want := map[string]string{"bg": "#1a1b26", "fg": "#c0caf5", "bar": "#16161e", "side": "#16161e", "card": "#1f2030", "border": "#101014", "rec": "#f7768e", "speaker": "#73daca"}
	for k, v := range want {
		if th.Tokens[k] != v {
			t.Errorf("%s = %s, want %s", k, th.Tokens[k], v)
		}
	}
	// 8-digit colours are flattened onto the background.
	if th.Tokens["sel"] != "#2d3650" {
		t.Errorf("sel = %s, want #7aa2f7 at 20%% over #1a1b26 = #2d3650", th.Tokens["sel"])
	}
	// No button colours: the accent comes from focusBorder (flattened) and on-accent is derived.
	if th.Tokens["accent"] != "#6787cd" {
		t.Errorf("accent = %s, want focusBorder flattened, #6787cd", th.Tokens["accent"])
	}
	// The dim description colour is lifted until it reads.
	if th.Tokens["fg2"] == "#2a2c3d" {
		t.Error("the unreadable description colour was kept")
	}
	checkComplete(t, th)
	checkReadable(t, th)

	named, _ := FromVSCode([]byte(darkVSCode), "My Night")
	if named.Name != "My Night" {
		t.Errorf("name override: %q", named.Name)
	}
}

func TestFromVSCodeLight(t *testing.T) {
	th, err := FromVSCode([]byte(lightVSCode), "")
	if err != nil {
		t.Fatal(err)
	}
	if th.Mode != "light" {
		t.Fatalf("mode %q", th.Mode)
	}
	want := map[string]string{"bg": "#fafafa", "fg": "#383a42", "side": "#f0f0f1", "bar": "#f0f0f1", "card": "#ffffff", "border": "#e5e5e6", "accent": "#4078f2", "on-accent": "#ffffff"}
	for k, v := range want {
		if th.Tokens[k] != v {
			t.Errorf("%s = %s, want %s", k, th.Tokens[k], v)
		}
	}
	// The error red is darkened until it reads on its pale pill, not replaced by Oscilloscope's.
	if r := th.Tokens["rec"]; r == "#e45649" || r == OscilloscopeLight.Tokens["rec"] {
		t.Errorf("rec = %s, want #e45649 darkened", r)
	}
	if th.Tokens["fg2"] == "#c8c8c8" || th.Tokens["fg3"] == "#a0a1a7" {
		t.Errorf("low-contrast text kept: fg2 %s fg3 %s", th.Tokens["fg2"], th.Tokens["fg3"])
	}
	checkComplete(t, th)
	checkReadable(t, th)
	if err := Validate(&th); err != nil {
		t.Errorf("an imported theme must validate: %v", err)
	}
}

func TestFromVSCodeModeFromBackground(t *testing.T) {
	th, err := FromVSCode([]byte(`{"colors":{"editor.background":"#ffffff","editor.foreground":"#333333"}}`), "Plain")
	if err != nil {
		t.Fatal(err)
	}
	if th.Mode != "light" {
		t.Errorf("mode %q, want light from the background", th.Mode)
	}
	checkComplete(t, th)
	checkReadable(t, th)
}

func TestFromVSCodeRejectsOtherFiles(t *testing.T) {
	for _, in := range []string{`{"name":"x","version":"1.0.0"}`, `{"colors":{}}`, `[1,2]`, `{"colors":{"tab.border":"#000000"}}`} {
		_, err := FromVSCode([]byte(in), "")
		if !errors.Is(err, ErrNotTheme) {
			t.Errorf("%s: want ErrNotTheme, got %v", in, err)
		}
	}
	for _, in := range []string{`not json`, `{"colors": {"editor.background": "#000000"`, `{"colors": }`} {
		_, err := FromVSCode([]byte(in), "")
		if err == nil || !strings.HasPrefix(err.Error(), "This file is not valid JSON") {
			t.Errorf("%s: want the invalid JSON error, got %v", in, err)
		}
	}
	if !strings.Contains(ErrNotTheme.Error(), "VS Code colour theme") {
		t.Errorf("unclear error %q", ErrNotTheme)
	}
}

func TestParseReadsAnExportBack(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	raw, err := s.Export(OscilloscopeDark.ID)
	if err != nil {
		t.Fatal(err)
	}
	th, err := Parse(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != OscilloscopeDark.Name || th.Mode != "dark" {
		t.Errorf("got %q %q", th.Name, th.Mode)
	}
	for k, v := range OscilloscopeDark.Tokens {
		if th.Tokens[k] != v {
			t.Errorf("%s = %s, want %s", k, th.Tokens[k], v)
		}
	}
	vs, err := Parse([]byte(lightVSCode), "")
	if err != nil || vs.Name != "Paper Light" {
		t.Errorf("a VS Code theme through Parse: %q %v", vs.Name, err)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "themes")}
	list, err := s.List()
	if err != nil || len(list) != len(Builtins) {
		t.Fatalf("a fresh store lists %d themes, %v", len(list), err)
	}
	imported, err := FromVSCode([]byte(darkVSCode), "")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Save(imported)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != "acme-midnight" || saved.Builtin {
		t.Fatalf("saved as %q builtin %v", saved.ID, saved.Builtin)
	}
	again, err := s.Save(imported)
	if err != nil || again.ID != "acme-midnight-2" {
		t.Fatalf("a second import with the same name: %q %v", again.ID, err)
	}
	named := imported
	named.Name = "Oscilloscope Dark"
	copyOf, err := s.Save(named)
	if err != nil || copyOf.ID != "oscilloscope-dark-2" {
		t.Fatalf("a theme named like a built-in: %q %v", copyOf.ID, err)
	}

	list, _ = s.List()
	var ids []string
	for _, th := range list {
		ids = append(ids, th.ID)
	}
	if got := strings.Join(ids, " "); got != "system oscilloscope-dark oscilloscope-light acme-midnight acme-midnight-2 oscilloscope-dark-2" {
		t.Errorf("list order: %s", got)
	}

	got, err := s.Get("acme-midnight")
	if err != nil || got.Name != "Acme Midnight" || got.Tokens["bg"] != "#1a1b26" {
		t.Fatalf("get: %+v %v", got, err)
	}
	raw, err := s.Export("acme-midnight")
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range got.Tokens {
		if back.Tokens[k] != v {
			t.Errorf("export round trip: %s = %s, want %s", k, back.Tokens[k], v)
		}
	}
	if _, err := s.Export(SystemID); err == nil {
		t.Error("the system theme has no colours to export")
	}

	if err := s.Delete("acme-midnight"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("acme-midnight"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if err := s.Delete("acme-midnight"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
	if err := s.Delete("../x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a path as id: %v", err)
	}
	bad := Theme{Name: "X", Mode: "dark", Tokens: map[string]string{"nope": "#000"}}
	if _, err := s.Save(bad); err == nil || !strings.Contains(err.Error(), "not a theme token") {
		t.Errorf("unknown token: %v", err)
	}
}

func TestStoreBuiltinsAreReadOnly(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	for _, b := range Builtins {
		th := clone(b)
		th.Mode = "dark"
		th.Tokens = clone(OscilloscopeDark).Tokens
		if _, err := s.Save(th); err == nil {
			t.Errorf("saving over %s must fail", b.ID)
		}
		if err := s.Delete(b.ID); err == nil {
			t.Errorf("deleting %s must fail", b.ID)
		}
	}
	// A file named like a built-in does not replace it.
	if err := os.WriteFile(filepath.Join(dir, "oscilloscope-dark.json"), []byte(`{"name":"Fake","mode":"dark","tokens":{"bg":"#ff0000"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("oscilloscope-dark")
	list, _ := s.List()
	if got.Name != "Oscilloscope Dark" || len(list) != len(Builtins) {
		t.Errorf("a file shadowed a built-in: %q, %d themes", got.Name, len(list))
	}
	// A broken file does not hide the others.
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if list, err := s.List(); err != nil || len(list) != len(Builtins) {
		t.Errorf("broken file: %d %v", len(list), err)
	}
}

func TestSelection(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "themes")}
	if got := s.Selected(); got.ID != SystemID {
		t.Fatalf("default %q, want system", got.ID)
	}
	if err := s.SetSelected("oscilloscope-light"); err != nil {
		t.Fatal(err)
	}
	if got := s.Selected(); got.ID != "oscilloscope-light" || got.Tokens["bg"] != "#f4f3f0" {
		t.Errorf("selected %q", got.ID)
	}
	if err := s.SetSelected("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown id: %v", err)
	}
	th, _ := FromVSCode([]byte(lightVSCode), "")
	saved, err := s.Save(th)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSelected(saved.ID); err != nil {
		t.Fatal(err)
	}
	if got := s.Selected(); got.ID != saved.ID {
		t.Errorf("selected %q, want %q", got.ID, saved.ID)
	}
	if err := s.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	if got := s.Selected(); got.ID != SystemID {
		t.Errorf("after deleting the chosen theme: %q, want system", got.ID)
	}
	// A selected file naming a theme that is gone falls back too.
	if err := os.WriteFile(filepath.Join(s.Dir, "selected"), []byte("gone\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.Selected(); got.ID != SystemID {
		t.Errorf("an unknown stored id: %q, want system", got.ID)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Acme Midnight": "acme-midnight", "  !!": "theme", "Café · Noir 2": "caf-noir-2"} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
