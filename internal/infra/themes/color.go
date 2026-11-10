package themes

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// rgba is a colour with channels from 0 to 255 and an alpha from 0 to 1.
type rgba struct {
	r, g, b float64
	a       float64
}

var (
	white = rgba{255, 255, 255, 1}
	black = rgba{0, 0, 0, 1}
)

// parseHex reads #rgb, #rgba, #rrggbb or #rrggbbaa.
func parseHex(s string) (rgba, error) {
	s = strings.TrimSpace(s)
	bad := fmt.Errorf("%q is not a hex colour such as #1d2330", s)
	if !strings.HasPrefix(s, "#") {
		return rgba{}, bad
	}
	h := s[1:]
	if len(h) == 3 || len(h) == 4 {
		var b strings.Builder
		for _, c := range h {
			b.WriteRune(c)
			b.WriteRune(c)
		}
		h = b.String()
	}
	if len(h) != 6 && len(h) != 8 {
		return rgba{}, bad
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgba{}, bad
	}
	c := rgba{a: 1}
	if len(h) == 8 {
		c.a = float64(v&0xff) / 255
		v >>= 8
	}
	c.r, c.g, c.b = float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	return c, nil
}

func (c rgba) hex() string {
	cl := func(x float64) int { return int(math.Round(math.Max(0, math.Min(255, x)))) }
	return fmt.Sprintf("#%02x%02x%02x", cl(c.r), cl(c.g), cl(c.b))
}

// over flattens c onto an opaque background.
func (c rgba) over(bg rgba) rgba {
	return rgba{r: c.r*c.a + bg.r*(1-c.a), g: c.g*c.a + bg.g*(1-c.a), b: c.b*c.a + bg.b*(1-c.a), a: 1}
}

// mix returns a blend: t=0 is a, t=1 is b.
func mix(a, b rgba, t float64) rgba {
	return rgba{r: a.r + (b.r-a.r)*t, g: a.g + (b.g-a.g)*t, b: a.b + (b.b-a.b)*t, a: 1}
}

func (c rgba) luminance() float64 {
	ch := func(x float64) float64 {
		x /= 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.r) + 0.7152*ch(c.g) + 0.0722*ch(c.b)
}

func contrast(a, b rgba) float64 {
	la, lb := a.luminance(), b.luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// minContrast is the lowest contrast of c against any of bgs.
func minContrast(c rgba, bgs ...rgba) float64 {
	m := math.Inf(1)
	for _, b := range bgs {
		m = math.Min(m, contrast(c, b))
	}
	return m
}

// extreme is white on a dark background and black on a light one.
func extreme(bg rgba) rgba {
	if contrast(white, bg) >= contrast(black, bg) {
		return white
	}
	return black
}

// readable moves c toward toward (usually the theme's text colour) until it reaches the contrast
// ratio min against every one of bgs, and on toward white or black when toward is not enough, so
// a colour stays legible while keeping as much of its hue as it can.
func readable(c, toward rgba, min float64, bgs ...rgba) rgba {
	c = c.rounded()
	if len(bgs) == 0 || minContrast(c, bgs...) >= min {
		return c
	}
	for i := 1; i <= 20; i++ {
		if x := mix(c, toward, float64(i)/20).rounded(); minContrast(x, bgs...) >= min {
			return x
		}
	}
	end := extreme(bgs[0])
	for i := 1; i <= 20; i++ {
		if x := mix(toward, end, float64(i)/20).rounded(); minContrast(x, bgs...) >= min {
			return x
		}
	}
	return end
}

// rounded is c as its #rrggbb form stores it, so a contrast checked on it holds once saved.
func (c rgba) rounded() rgba {
	r, _ := parseHex(c.hex())
	return r
}

// NormalizeHex returns a colour as lowercase #rrggbb, flattening any alpha onto bg.
func NormalizeHex(s string, bg string) (string, error) {
	c, err := parseHex(s)
	if err != nil {
		return "", err
	}
	if c.a < 1 {
		b, err := parseHex(bg)
		if err != nil {
			b = white
		}
		c = c.over(b)
	}
	return c.hex(), nil
}
