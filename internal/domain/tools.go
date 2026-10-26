package domain

// The names Claude Code gives Notted's two tools (server "notted").
const (
	AskToolName        = "mcp__notted__ask"
	PermissionToolName = "mcp__notted__permission"
	// ToolTimeoutEnv keeps a pending form from timing out in Claude Code (24 hours, in ms).
	ToolTimeoutEnv = "MCP_TOOL_TIMEOUT=86400000"
)
