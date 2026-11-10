package themes

// SystemID is the default theme: tokens.css as it is, following macOS light and dark.
const SystemID = "system"

// System has no tokens of its own: it leaves tokens.css to follow macOS.
var System = Theme{ID: SystemID, Name: "Oscilloscope (match system)", Mode: "system", Builtin: true, Tokens: map[string]string{}}

// OscilloscopeDark and OscilloscopeLight are the colours of tokens.css; a test keeps them in step.
var OscilloscopeDark = Theme{ID: "oscilloscope-dark", Name: "Oscilloscope Dark", Mode: "dark", Builtin: true, Tokens: map[string]string{
	"bg": "#111214", "bar": "#0b0c0d", "side": "#0b0c0d", "card": "#18191c", "fg": "#ecebe7", "fg2": "#b6b3ab", "fg3": "#8d8a83", "fg3-text": "#9a978f",
	"border": "#2a2b2f", "sel": "#2c2416", "dot": "#3a3b40", "accent": "#f2a93b", "on-accent": "#1b1204", "rec": "#ff7b6b", "rec-bg": "#3b1d18", "wave": "#a8742a", "speaker": "#f2b55a",
}}

var OscilloscopeLight = Theme{ID: "oscilloscope-light", Name: "Oscilloscope Light", Mode: "light", Builtin: true, Tokens: map[string]string{
	"bg": "#f4f3f0", "bar": "#ebe9e4", "side": "#ebe9e4", "card": "#fbfaf8", "fg": "#1b1a17", "fg2": "#4f4b43", "fg3": "#736e64", "fg3-text": "#6a655b",
	"border": "#dcd8d0", "sel": "#f3e3c6", "dot": "#cdc8be", "accent": "#9a5600", "on-accent": "#ffffff", "rec": "#b8321f", "rec-bg": "#f5e0d9", "wave": "#d8a75e", "speaker": "#9a5600",
}}

// Builtins are the read-only themes Phonon ships, the default first.
var Builtins = []Theme{System, OscilloscopeDark, OscilloscopeLight}

// WindowBackground is the colour the native window shows before the page paints: the chosen
// theme's bg, or for the system theme the Oscilloscope variant macOS currently asks for.
func WindowBackground(t Theme, systemDark bool) (r, g, b uint8) {
	bg := t.Tokens["bg"]
	if t.ID == SystemID || bg == "" {
		bg = OscilloscopeLight.Tokens["bg"]
		if systemDark {
			bg = OscilloscopeDark.Tokens["bg"]
		}
	}
	c, err := parseHex(bg)
	if err != nil {
		c, _ = parseHex(OscilloscopeDark.Tokens["bg"])
	}
	return uint8(c.r), uint8(c.g), uint8(c.b)
}
