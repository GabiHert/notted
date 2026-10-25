package menubar

import (
	"testing"
	"time"
)

func TestDispatchRunsTheMatchingHandler(t *testing.T) {
	got := make(chan string, 4)
	i := New()
	i.Install(Handlers{
		Start: func() { got <- "start" },
		Stop:  func() { got <- "stop" },
		Open:  func() { got <- "open" },
		Quit:  func() { got <- "quit" },
	})
	for action, want := range map[int]string{actionStart: "start", actionStop: "stop", actionOpen: "open", actionQuit: "quit"} {
		dispatchActive(action)
		select {
		case g := <-got:
			if g != want {
				t.Fatalf("action %d ran %q, want %q", action, g, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("action %d ran nothing", action)
		}
	}
}

func TestDispatchIgnoresUnknownActionsAndMissingHandlers(t *testing.T) {
	i := New()
	i.Install(Handlers{})
	dispatchActive(actionStart)
	dispatchActive(99)
	i.SetRecording(true)
	i.SetRecording(false)
}
