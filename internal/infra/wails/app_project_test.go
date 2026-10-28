package wails

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/GabiHert/notted/internal/domain"
)

func TestProjectDeleteDTOsNeverCarryNullSlices(t *testing.T) {
	b, err := json.Marshal(toProjectDeletePlanDTO(domain.ProjectDeletePlan{Key: "acme"}))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); strings.Contains(s, "null") || !strings.Contains(s, `"files":[]`) || !strings.Contains(s, `"folders":[]`) {
		t.Fatalf("plan json = %s", s)
	}
	b, err = json.Marshal(toProjectDeleteResultDTO(domain.ProjectDeleteResult{DeletedMeetings: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); strings.Contains(s, "null") || !strings.Contains(s, `"trackedRemoved":[]`) {
		t.Fatalf("result json = %s", s)
	}
}

func TestProjectDeletePlanDTORoundTrip(t *testing.T) {
	p := domain.ProjectDeletePlan{Key: "acme", Files: []string{"a", "b"}, Folders: []string{"d/"}, LocalMeetings: 3, UnsavedMeetings: 1, Recording: true}
	if got := fromProjectDeletePlanDTO(toProjectDeletePlanDTO(p)); !reflect.DeepEqual(got, p) {
		t.Fatalf("round trip = %+v, want %+v", got, p)
	}
}
