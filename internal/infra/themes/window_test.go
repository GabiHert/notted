package themes

import "testing"

func TestWindowBackground(t *testing.T) {
	custom := Theme{ID: "solar", Mode: "light", Tokens: map[string]string{"bg": "#fdf6e3"}}
	cases := []struct {
		name    string
		t       Theme
		dark    bool
		r, g, b uint8
	}{
		{"system, macOS dark", System, true, 0x11, 0x12, 0x14},
		{"system, macOS light", System, false, 0xf4, 0xf3, 0xf0},
		{"fixed light ignores macOS", OscilloscopeLight, true, 0xf4, 0xf3, 0xf0},
		{"custom theme", custom, true, 0xfd, 0xf6, 0xe3},
	}
	for _, c := range cases {
		r, g, b := WindowBackground(c.t, c.dark)
		if r != c.r || g != c.g || b != c.b {
			t.Errorf("%s: got #%02x%02x%02x", c.name, r, g, b)
		}
	}
}
