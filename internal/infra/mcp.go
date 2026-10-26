package infra

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/GabiHert/notted/internal/domain"
	"github.com/GabiHert/notted/internal/integration/askipc"
	"github.com/GabiHert/notted/internal/integration/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunMCP serves the ask and permission tools on stdio and returns the exit code.
// It never writes to stdout: that carries the protocol.
func RunMCP(args []string) int {
	return runMCP(context.Background(), args, &mcp.StdioTransport{}, os.Stderr)
}

func runMCP(ctx context.Context, args []string, t mcp.Transport, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	meeting := fs.String("meeting", "", "meeting id")
	socket := fs.String("socket", "", "app socket path")
	token := fs.String("token", "", "session token")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *meeting == "" || *socket == "" || *token == "" {
		fmt.Fprintln(stderr, "notted mcp: --meeting, --socket and --token are required")
		return 2
	}
	if !domain.ValidMeetingID(*meeting) {
		fmt.Fprintln(stderr, "notted mcp: the meeting id is not valid")
		return 2
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := mcpserver.New(askipc.Client{Socket: *socket, MeetingID: *meeting, Token: *token})
	err := srv.Run(ctx, t)
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return 0
	}
	fmt.Fprintf(stderr, "notted mcp: %v\n", err)
	return 1
}
