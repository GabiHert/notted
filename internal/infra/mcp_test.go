package infra

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunMCPMissingFlags(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--meeting", "m1", "--socket", "/tmp/x"},
		{"--socket", "/tmp/x", "--token", "t"},
		{"--meeting", "m1", "--token", "t"},
	} {
		var stderr bytes.Buffer
		ct, _ := mcp.NewInMemoryTransports()
		if code := runMCP(context.Background(), args, ct, &stderr); code != 2 {
			t.Fatalf("%v: code %d", args, code)
		}
		if !strings.Contains(stderr.String(), "notted mcp: --meeting, --socket and --token are required") {
			t.Fatalf("stderr %q", stderr.String())
		}
	}
}

func TestRunMCPParseError(t *testing.T) {
	var stderr bytes.Buffer
	ct, _ := mcp.NewInMemoryTransports()
	if code := runMCP(context.Background(), []string{"--bogus"}, ct, &stderr); code != 2 {
		t.Fatalf("code %d", code)
	}
}

func TestRunMCPInvalidMeetingID(t *testing.T) {
	var stderr bytes.Buffer
	ct, _ := mcp.NewInMemoryTransports()
	code := runMCP(context.Background(), []string{"--meeting", "../x", "--socket", "/tmp/s", "--token", "t"}, ct, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "notted mcp: the meeting id is not valid") {
		t.Fatalf("code %d stderr %q", code, stderr.String())
	}
}

func TestRunMCPServesToolsList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ct, st := mcp.NewInMemoryTransports()
	codes := make(chan int, 1)
	var stderr bytes.Buffer
	go func() {
		codes <- runMCP(ctx, []string{"--meeting", "m1", "--socket", "/tmp/none.sock", "--token", "t"}, st, &stderr)
	}()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 2 {
		t.Fatalf("tools %d", len(res.Tools))
	}
	cs.Close()
	if code := <-codes; code != 0 {
		t.Fatalf("code %d stderr %q", code, stderr.String())
	}
}
