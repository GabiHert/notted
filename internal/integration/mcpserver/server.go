// Package mcpserver is the MCP tool server `Notted mcp` runs for Claude Code:
// the ask tool and the permission prompt tool.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const askDescription = `Show the person a form in the Notted app and wait for the answer. Use it whenever you need the person's choice, a yes or no, or a short text. Pieces: text (Markdown), html (static HTML; scripts and network loads are blocked), choice (one option), multi (any number of options), input (a text field), confirm (yes or no). The result is JSON: status is answered or dismissed; answers maps each answerable piece id to the chosen label, the list of checked labels, the text, or yes or no; note is the person's free text. If status is dismissed, do not act on the form.`

const permissionDescription = `Notted's permission prompt for Claude Code. Claude Code calls it; do not call it yourself.`

const askSchema = `{"type":"object","required":["title","pieces"],"additionalProperties":false,"properties":{"title":{"type":"string","description":"One line shown at the top of the card."},"pieces":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","required":["kind","id"],"properties":{"kind":{"type":"string","enum":["text","html","choice","multi","input","confirm"]},"id":{"type":"string","description":"Unique in the form: lowercase letters, digits, dashes or underscores. The key of this piece's answer."},"markdown":{"type":"string","description":"text only."},"html":{"type":"string","description":"html only. Static HTML; scripts and network loads are blocked."},"question":{"type":"string","description":"choice, multi, input and confirm."},"options":{"type":"array","description":"choice: 2 to 10. multi: 1 to 50.","items":{"type":"object","required":["label"],"properties":{"label":{"type":"string"},"description":{"type":"string"},"recommended":{"type":"boolean","description":"choice only; at most one."}}}},"placeholder":{"type":"string","description":"input only."},"multiline":{"type":"boolean","description":"input only."},"required":{"type":"boolean","description":"input only."},"yes_label":{"type":"string","description":"confirm only; default Yes."},"no_label":{"type":"string","description":"confirm only; default No."}}}}}}`

const permissionSchema = `{"type":"object","required":["tool_name","input"],"properties":{"tool_name":{"type":"string"},"input":{"type":"object"},"tool_use_id":{"type":"string"}}}`

type handler = func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error)

// New returns the MCP server with the ask and permission tools, answering through asker.
func New(asker port.FormAsker) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "notted", Version: "1.0.0"}, nil)
	srv.AddTool(&mcp.Tool{Name: "ask", Description: askDescription, InputSchema: json.RawMessage(askSchema)}, askHandler(asker))
	srv.AddTool(&mcp.Tool{Name: "permission", Description: permissionDescription, InputSchema: json.RawMessage(permissionSchema)}, permissionHandler(asker))
	return srv
}

func textResult(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func askHandler(asker port.FormAsker) handler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		form, err := domain.ParseForm(req.Params.Arguments)
		if err != nil {
			return textResult(err.Error(), true), nil
		}
		ans, err := asker.Ask(ctx, domain.FormAsk, form)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, domain.ErrAppNotRunning) {
			return textResult("Notted is not running. Open Notted, then ask again.", true), nil
		}
		if err != nil {
			return textResult(err.Error(), true), nil
		}
		return textResult(domain.EncodeAnswer(ans), false), nil
	}
}

func permissionHandler(asker port.FormAsker) handler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var in struct {
			ToolName  string          `json:"tool_name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &in); err != nil {
			return textResult(domain.PermissionDeny("Notted could not read this permission request."), false), nil
		}
		form, err := domain.PermissionForm(in.ToolName, in.Input)
		if err != nil {
			return textResult(domain.PermissionDeny("Notted could not read this permission request."), false), nil
		}
		ans, err := asker.Ask(ctx, domain.FormPermission, form)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, domain.ErrAppNotRunning) {
			return textResult(domain.PermissionDeny("Notted is not running, so nobody could answer this request."), false), nil
		}
		if err != nil {
			return textResult(domain.PermissionDeny("Notted could not show this request: "+err.Error()), false), nil
		}
		return textResult(domain.PermissionResult(in.Input, ans), false), nil
	}
}
