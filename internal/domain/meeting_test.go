package domain

import "testing"

func TestValidMeetingID(t *testing.T) {
	for id, want := range map[string]bool{
		"0b0e6f0e-6a53-4b8b-9c0e-2f1d2a3b4c5d": true,
		"m1":                                   true,
		"":                                     false,
		".":                                    false,
		"..":                                   false,
		"../x":                                 false,
		"a/b":                                  false,
		`a\b`:                                  false,
		"a b":                                  false,
		"-rf":                                  false,
		"a\x00b":                               false,
	} {
		if got := ValidMeetingID(id); got != want {
			t.Errorf("ValidMeetingID(%q) = %v, want %v", id, got, want)
		}
	}
}
