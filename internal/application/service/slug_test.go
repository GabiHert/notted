package service

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	if got := slugify("Reunião de Alinhamento — Q4!"); got != "reuniao-de-alinhamento-q4" {
		t.Fatalf("got %q", got)
	}
	if got := slugify("!!! ???"); got != "meeting" {
		t.Fatalf("got %q", got)
	}
	got := slugify(strings.Repeat("abcd ", 18))
	if len(got) > 60 || strings.HasSuffix(got, "-") || len(got) < 55 {
		t.Fatalf("got %q (%d)", got, len(got))
	}
}
