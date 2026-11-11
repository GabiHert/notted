package wails

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/phonon/internal/update"
)

func TestToUpdateStatusMapsTheCardFields(t *testing.T) {
	date := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	d := toUpdateStatus(update.Status{Version: "v0.1.0", Repo: "GabiHert/notted", UpToDate: false, HasToken: true,
		State: "idle", Latest: &update.Release{Tag: "v0.2.0", Name: "Phonon v0.2.0", Notes: "n", Date: date, URL: "u"},
		InstalledApp: "/Applications/Phonon.app", Seq: 3})
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version":"v0.1.0"`, `"repo":"GabiHert/notted"`, `"upToDate":false`, `"hasToken":true`,
		`"tag":"v0.2.0"`, `"date":1794398400000`, `"lines":[]`, `"installedApp":"/Applications/Phonon.app"`, `"seq":3`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
	if toUpdateStatus(update.Status{}).Latest != nil {
		t.Fatal("latest must be null before a check")
	}
}
