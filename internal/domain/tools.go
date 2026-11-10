package domain

// The names Claude Code gives Phonon's two tools (server "phonon").
const (
	AskToolName        = "mcp__phonon__ask"
	PermissionToolName = "mcp__phonon__permission"
	// ToolTimeoutEnv keeps a pending form from timing out in Claude Code (24 hours, in ms).
	ToolTimeoutEnv = "MCP_TOOL_TIMEOUT=86400000"
)
