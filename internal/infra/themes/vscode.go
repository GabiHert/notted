package themes

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// StripJSONC removes // and /* */ comments and trailing commas, leaving strings intact, so VS Code's
// JSON-with-comments files parse as JSON.
func StripJSONC(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inStr, esc := false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inStr {
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}
	// Trailing commas: a comma followed only by whitespace and then } or ].
	res := make([]byte, 0, len(out))
	inStr, esc = false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inStr {
			res = append(res, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
		}
		if c == ',' {
			j := i + 1
			for j < len(out) && strings.ContainsRune(" \t\r\n", rune(out[j])) {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				continue
			}
		}
		res = append(res, c)
	}
	return res
}

// ErrNotTheme is returned for a file that is not a VS Code colour theme.
var ErrNotTheme = errors.New("This file is not a VS Code colour theme: it needs a \"colors\" object with keys such as editor.background and editor.foreground.")

type vscodeTheme struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Colors map[string]string `json:"colors"`
}

// vscodeKeys maps each Phonon token to the VS Code colour keys it is read from, first match wins.
// Tokens left out (fg3-text, dot, rec-bg, wave) and keys a theme does not set are derived.
var vscodeKeys = []struct {
	Token string
	Keys  []string
}{
	{"fg", []string{"editor.foreground", "foreground"}},
	{"bar", []string{"titleBar.activeBackground", "sideBar.background"}},
	{"side", []string{"sideBar.background", "activityBar.background"}},
	{"card", []string{"editorWidget.background", "dropdown.background", "editorHoverWidget.background"}},
	{"fg2", []string{"descriptionForeground", "sideBarTitle.foreground"}},
	{"fg3", []string{"editorLineNumber.foreground", "disabledForeground"}},
	{"border", []string{"panel.border", "editorGroup.border", "sideBar.border", "widget.border", "contrastBorder"}},
	{"sel", []string{"list.activeSelectionBackground", "editor.selectionBackground"}},
	{"accent", []string{"button.background", "focusBorder", "textLink.foreground"}},
	{"on-accent", []string{"button.foreground"}},
	{"rec", []string{"errorForeground", "editorError.foreground", "terminal.ansiRed"}},
	{"speaker", []string{"textLink.foreground"}},
}

// FromVSCode maps a VS Code colour theme onto Phonon's tokens. name overrides the theme's own name.
// Missing tokens are derived, and every text colour is kept readable on its background.
func FromVSCode(data []byte, name string) (Theme, error) {
	var v vscodeTheme
	if err := json.Unmarshal(StripJSONC(data), &v); err != nil {
		var se *json.SyntaxError
		if errors.As(err, &se) {
			return Theme{}, fmt.Errorf("This file is not valid JSON (at byte %d): %v", se.Offset, err)
		}
		return Theme{}, ErrNotTheme
	}
	if len(v.Colors) == 0 || (v.Colors["editor.background"] == "" && v.Colors["editor.foreground"] == "" && v.Colors["foreground"] == "") {
		return Theme{}, ErrNotTheme
	}
	t := Theme{Name: strings.TrimSpace(name), Tokens: map[string]string{}}
	if t.Name == "" {
		t.Name = strings.TrimSpace(v.Name)
	}
	if t.Name == "" {
		t.Name = "Imported theme"
	}
	if len(t.Name) > 60 {
		t.Name = strings.TrimSpace(t.Name[:60])
	}
	pick := func(keys ...string) (string, bool) {
		for _, k := range keys {
			if s := strings.TrimSpace(v.Colors[k]); s != "" {
				if _, err := parseHex(s); err == nil {
					return s, true
				}
			}
		}
		return "", false
	}
	bg, ok := pick("editor.background", "sideBar.background", "panel.background")
	if !ok {
		bg = "#1e1e1e"
		if strings.Contains(strings.ToLower(v.Type), "light") {
			bg = "#ffffff"
		}
	}
	bgc, _ := parseHex(bg)
	bgc = bgc.over(white)
	bg = bgc.hex()
	switch strings.ToLower(v.Type) {
	case "light", "hclight", "hc-light", "vs":
		t.Mode = "light"
	case "dark", "hc", "hcdark", "hc-black", "vs-dark":
		t.Mode = "dark"
	default:
		if bgc.luminance() > 0.4 {
			t.Mode = "light"
		} else {
			t.Mode = "dark"
		}
	}
	t.Tokens["bg"] = bg
	for _, m := range vscodeKeys {
		if s, ok := pick(m.Keys...); ok {
			h, _ := NormalizeHex(s, bg)
			t.Tokens[m.Token] = h
		}
	}
	if _, ok := t.Tokens["fg"]; !ok {
		t.Tokens["fg"] = OscilloscopeLight.Tokens["fg"]
		if t.Mode == "dark" {
			t.Tokens["fg"] = OscilloscopeDark.Tokens["fg"]
		}
	}
	Complete(&t)
	ensureReadable(&t)
	return t, nil
}
