// Package themes holds Phonon's colour themes: the built-in Oscilloscope ones, custom ones saved as
// JSON files in the data folder's themes folder, and an importer for VS Code colour themes. A theme
// sets every colour token of frontend/src/styles/tokens.css (without the leading --).
package themes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Theme is one colour theme. The system theme has mode "system" and no tokens: it leaves
// tokens.css to follow macOS.
type Theme struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Mode    string            `json:"mode"`
	Builtin bool              `json:"builtin"`
	Tokens  map[string]string `json:"tokens"`
}

// TokenNames is every colour token a theme sets, in the order tokens.css lists them.
var TokenNames = []string{"bg", "bar", "side", "card", "fg", "fg2", "fg3", "fg3-text", "border", "sel", "dot", "accent", "on-accent", "rec", "rec-bg", "wave", "speaker"}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

// Slug turns a name into an id: lowercase letters, digits and dashes.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteRune('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		s = "theme"
	}
	return s
}

// Validate checks a custom theme and normalises its colours to #rrggbb. Missing tokens are derived.
func Validate(t *Theme) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return fmt.Errorf("A theme needs a name")
	}
	if len(t.Name) > 60 {
		return fmt.Errorf("A theme name may have at most 60 characters")
	}
	if t.Mode != "light" && t.Mode != "dark" {
		return fmt.Errorf("The mode must be light or dark, got %q", t.Mode)
	}
	known := map[string]bool{}
	for _, n := range TokenNames {
		known[n] = true
	}
	bg := t.Tokens["bg"]
	out := map[string]string{}
	for k, v := range t.Tokens {
		if !known[k] {
			return fmt.Errorf("%q is not a theme token; tokens are %s", k, strings.Join(TokenNames, ", "))
		}
		h, err := NormalizeHex(v, bg)
		if err != nil {
			return fmt.Errorf("Token %s: %w", k, err)
		}
		out[k] = h
	}
	t.Tokens = out
	Complete(t)
	return nil
}

// Complete fills every missing token from the ones that are set, and from the Oscilloscope theme
// of the same mode when there is nothing to derive from.
func Complete(t *Theme) {
	if t.Tokens == nil {
		t.Tokens = map[string]string{}
	}
	base := OscilloscopeLight
	if t.Mode == "dark" {
		base = OscilloscopeDark
	}
	has := func(k string) bool { _, ok := t.Tokens[k]; return ok }
	get := func(k string) rgba {
		c, err := parseHex(t.Tokens[k])
		if err != nil {
			c, _ = parseHex(base.Tokens[k])
		}
		return c
	}
	set := func(k string, c rgba) {
		if !has(k) {
			t.Tokens[k] = c.hex()
		}
	}
	if has("bg") || has("fg") {
		bg, fg := get("bg"), get("fg")
		set("side", mix(bg, fg, 0.04))
		set("card", mix(bg, fg, 0.03))
		set("border", mix(bg, fg, 0.13))
		set("dot", mix(bg, fg, 0.2))
		set("fg2", mix(fg, bg, 0.28))
		set("fg3", mix(fg, bg, 0.45))
	}
	if has("side") {
		set("bar", get("side"))
	}
	if has("fg3") || has("bg") {
		bg, fg := get("bg"), get("fg")
		set("fg3-text", readable(get("fg3"), fg, 4.5, bg, get("side"), get("card")))
	}
	if has("accent") {
		bg, acc := get("bg"), get("accent")
		set("sel", mix(bg, acc, 0.18))
		set("wave", mix(acc, bg, 0.35))
		set("speaker", readable(acc, get("fg"), 4.5, bg))
		set("on-accent", onColour(acc))
	}
	if has("rec") {
		set("rec-bg", mix(get("bg"), get("rec"), 0.16))
	}
	for _, k := range TokenNames {
		if !has(k) {
			t.Tokens[k] = base.Tokens[k]
		}
	}
}

// onColour is the text colour for a filled accent: white, or a near-black tinted with the accent.
func onColour(acc rgba) rgba {
	if contrast(white, acc) >= contrast(black, acc) {
		return white
	}
	return mix(black, acc, 0.1).rounded()
}

// ensureReadable adjusts an imported theme's colours so every pair the app draws stays legible:
// text at 4.5:1, the accent at 3:1 against the page and the selected row, and the accent's own text
// at 3:1 on it.
func ensureReadable(t *Theme) {
	c := func(k string) rgba { x, _ := parseHex(t.Tokens[k]); return x }
	put := func(k string, x rgba) { t.Tokens[k] = x.hex() }
	bg, side, card := c("bg"), c("side"), c("card")
	put("fg", readable(c("fg"), extreme(bg), 4.5, bg, card, side))
	fg := c("fg")
	put("fg2", readable(c("fg2"), fg, 4.5, bg))
	put("fg3", readable(c("fg3"), fg, 4.5, bg, side, card))
	put("fg3-text", readable(c("fg3-text"), fg, 4.5, bg, side, card))
	put("accent", readable(c("accent"), fg, 3, bg, c("sel")))
	acc := c("accent")
	if contrast(c("on-accent"), acc) < 3 {
		put("on-accent", readable(onColour(acc), extreme(acc), 4.5, acc))
	}
	put("speaker", readable(c("speaker"), fg, 4.5, bg))
	put("rec", readable(c("rec"), extreme(bg), 4.5, c("rec-bg"), bg))
}

// exported is the file Export writes and Parse reads back.
type exported struct {
	Name   string            `json:"name"`
	Mode   string            `json:"mode"`
	Tokens map[string]string `json:"tokens"`
}

// Parse reads a theme exported from Phonon (name, mode and tokens) or a VS Code colour theme. name,
// when set, overrides the file's own name.
func Parse(data []byte, name string) (Theme, error) {
	var p exported
	if json.Unmarshal(StripJSONC(data), &p) == nil && len(p.Tokens) > 0 && p.Mode != "" {
		t := Theme{Name: p.Name, Mode: p.Mode, Tokens: p.Tokens}
		if strings.TrimSpace(name) != "" {
			t.Name = name
		}
		if strings.TrimSpace(t.Name) == "" {
			t.Name = "Imported theme"
		}
		return t, Validate(&t)
	}
	return FromVSCode(data, name)
}
