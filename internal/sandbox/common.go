package sandbox

import "github.com/mark3labs/mcp-go/mcp"

// withEntrypoint creates an entrypoint parameter for the tool.
func withEntrypoint(name string, description string) mcp.ToolOption {
	return mcp.WithString(name, mcp.Description(description))
}

// withAdditionalFiles creates a files parameter for the tool.
func withAdditionalFiles() mcp.ToolOption {
	return mcp.WithArray("files",
		mcp.Description("Files to be included in the sandbox"),
		mcp.Items(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filename": map[string]any{
					"type": "string",
				},
				"content": map[string]any{
					"type": "string",
				},
			},
		}),
	)
}

// withFile adds a file parameter to the tool.
func withFile(name string, description string, required bool) mcp.ToolOption {
	if required {
		return mcp.WithString(name, mcp.Description(description), mcp.Required())
	}
	return mcp.WithString(name, mcp.Description(description))
}

// withSession adds a session_id parameter for persistent file storage across calls.
func withSession() mcp.ToolOption {
	return mcp.WithString("session_id",
		mcp.Description("Optional session identifier for persistent file storage across calls. When provided, files persist between executions. Omit for one-shot ephemeral execution."),
	)
}

// withCleanup adds a cleanup boolean parameter to remove a session.
func withCleanup() mcp.ToolOption {
	return mcp.WithBoolean("cleanup",
		mcp.Description("When true and session_id is provided, removes the session and all its files after execution."),
	)
}

// withPatch adds a patch parameter for a specific file (unified diff format).
func withPatch(name string) mcp.ToolOption {
	return mcp.WithString(name+"_patch",
		mcp.Description("Unified diff patch to apply to `"+name+"` for incremental edits. Use instead of rewriting the entire file content."),
	)
}
