package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pottekkat/sandbox-mcp/internal/config"
)

// Global session manager (initialized in main.go)
var sessionManager *SessionManager

// SetSessionManager sets the global session manager instance
func SetSessionManager(sm *SessionManager) {
	sessionManager = sm
}

// waitForContainer waits for a container to be in running state with a specified timeout
func waitForContainer(ctx context.Context, cli *client.Client, containerID string, timeout time.Duration) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for container to start")
		case <-timeoutCh:
			return fmt.Errorf("container did not reach running state within %v", timeout)
		case <-ticker.C:
			inspect, err := cli.ContainerInspect(ctx, containerID)
			if err != nil {
				return fmt.Errorf("failed to inspect container: %v", err)
			}
			if inspect.State != nil && inspect.State.Running {
				return nil
			}
		}
	}
}

// NewSandboxTool creates a sandbox tool from a config
func NewSandboxTool(sandboxConfig *config.SandboxConfig) mcp.Tool {
	options := []mcp.ToolOption{
		mcp.WithDescription(generateSandboxDescription(sandboxConfig)),
		withEntrypoint(sandboxConfig.ParamEntrypoint(), fmt.Sprintf("Code to be stored in a file named `%s` and executed with the command `%s`.",
			sandboxConfig.Entrypoint,
			strings.Join(sandboxConfig.Command, " "))),

		mcp.WithTitleAnnotation(sandboxConfig.Name()),
		mcp.WithReadOnlyHintAnnotation(sandboxConfig.Hints.IsReadOnly(sandboxConfig.Mount.ReadOnly, sandboxConfig.Security.ReadOnly)),
		mcp.WithDestructiveHintAnnotation(sandboxConfig.Hints.IsDestructive()),
		mcp.WithIdempotentHintAnnotation(sandboxConfig.Hints.IsIdempotent()),
		mcp.WithOpenWorldHintAnnotation(sandboxConfig.Hints.IsExternalInteraction(sandboxConfig.Security.Network)),

		// Session and cleanup support
		withSession(),
		withCleanup(),
	}

	// Add any specific additional files if provided in the config
	for _, file := range sandboxConfig.Parameters.Files {
		options = append(options, withFile(file.ParamName(), file.Description, true))
		// Add patch parameter for each file
		options = append(options, withPatch(file.ParamName()))
	}

	// Add patch parameter for entrypoint
	options = append(options, withPatch(sandboxConfig.ParamEntrypoint()))

	// Allow adding more files if enabled
	if sandboxConfig.Parameters.AdditionalFiles {
		options = append(options, withAdditionalFiles())
	}

	return mcp.NewTool(sandboxConfig.Id, options...)
}

// NewSandboxToolHandler creates a handler function for a sandbox tool
func NewSandboxToolHandler(sandboxConfig *config.SandboxConfig) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Parse session and cleanup parameters
		sessionID, _ := request.Params.Arguments["session_id"].(string)
		cleanup, _ := request.Params.Arguments["cleanup"].(bool)
		useSession := sessionID != "" && sessionManager != nil

		// Resolve working directory
		var dir string
		if useSession {
			session, err := sessionManager.GetOrCreate(sessionID)
			if err != nil {
				return nil, fmt.Errorf("failed to get/create session: %v", err)
			}
			dir = session.Dir
			if cleanup {
				defer func() {
					if err := sessionManager.Remove(sessionID); err != nil {
						log.Printf("Failed to remove session %s: %v", sessionID, err)
					}
				}()
			}
		} else {
			var err error
			dir, err = os.MkdirTemp("", sandboxConfig.Mount.TmpDirPrefix)
			if err != nil {
				return nil, fmt.Errorf("failed to create a temporary directory: %v", err)
			}
			if err := os.Chmod(dir, 0777); err != nil {
				return nil, fmt.Errorf("failed to set temporary directory permissions: %v", err)
			}
			defer os.RemoveAll(dir)
		}

		// Process entrypoint file
		entrypointFile := config.SandboxFile{Name: sandboxConfig.Entrypoint}
		entrypointParam := entrypointFile.ParamName()
		entrypointContent, _ := request.Params.Arguments[entrypointParam].(string)
		entrypointPatch, _ := request.Params.Arguments[entrypointParam+"_patch"].(string)

		cmdFile := filepath.Join(dir, sandboxConfig.Entrypoint)
		if entrypointContent != "" {
			if err := os.WriteFile(cmdFile, []byte(entrypointContent), sandboxConfig.Mount.ScriptPerms()); err != nil {
				return nil, fmt.Errorf("failed to write command file: %v", err)
			}
		} else if entrypointPatch != "" {
			if err := ApplyPatch(cmdFile, entrypointPatch); err != nil {
				return nil, fmt.Errorf("failed to apply patch to %s: %v", sandboxConfig.Entrypoint, err)
			}
		} else if !useSession {
			return nil, fmt.Errorf("%s file is required", sandboxConfig.Entrypoint)
		} else {
			// Session mode: check if file exists
			if _, err := os.Stat(cmdFile); os.IsNotExist(err) {
				return nil, fmt.Errorf("%s file is required (not found in session %s)", sandboxConfig.Entrypoint, sessionID)
			}
		}

		// Process configured files
		for _, file := range sandboxConfig.Parameters.Files {
			paramName := file.ParamName()
			content, _ := request.Params.Arguments[paramName].(string)
			patchStr, _ := request.Params.Arguments[paramName+"_patch"].(string)

			filePath := filepath.Join(dir, file.Name)
			_, fileStatErr := os.Stat(filePath)
			isRequired := !useSession || fileStatErr != nil

			if content != "" {
				if err := os.WriteFile(filePath, []byte(content), sandboxConfig.Mount.ScriptPerms()); err != nil {
					return nil, fmt.Errorf("failed to write file %s: %v", file.Name, err)
				}
			} else if patchStr != "" {
				if err := ApplyPatch(filePath, patchStr); err != nil {
					return nil, fmt.Errorf("failed to apply patch to %s: %v", file.Name, err)
				}
			} else if isRequired {
				return nil, fmt.Errorf("%s file is required", file.Name)
			}
		}

		// Handle additional files
		if files, ok := request.Params.Arguments["files"].([]any); ok {
			for _, file := range files {
				if fileMap, ok := file.(map[string]any); ok {
					filename := fileMap["filename"].(string)
					content := fileMap["content"].(string)

					filePath := filepath.Join(dir, filename)
					if err := os.WriteFile(filePath, []byte(content), sandboxConfig.Mount.ScriptPerms()); err != nil {
						return nil, fmt.Errorf("failed to write file %s: %v", filename, err)
					}
				}
			}
		}

		// Initialize Docker client
		cli, err := client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create Docker client: %v", err)
		}
		defer cli.Close()

		// Build container labels
		labels := map[string]string{
			"sandbox-mcp": "true",
		}
		if useSession {
			labels["sandbox-mcp-session"] = sessionID
		}

		// Create container config
		containerConfig := &container.Config{
			Image:      sandboxConfig.Image,
			Cmd:        sandboxConfig.RunCommand(),
			WorkingDir: sandboxConfig.Mount.WorkDir,
			User:       sandboxConfig.User,
			Tty:        sandboxConfig.Tty(),
			Labels:     labels,
		}

		// Create host config
		hostConfig := &container.HostConfig{
			Resources: container.Resources{
				Memory:    sandboxConfig.Resources.Memory * 1024 * 1024,
				NanoCPUs:  int64(sandboxConfig.Resources.CPU * 1e9),
				PidsLimit: &sandboxConfig.Resources.Processes,
				Ulimits: []*container.Ulimit{
					{
						Name: "nofile",
						Soft: sandboxConfig.Resources.Files,
						Hard: sandboxConfig.Resources.Files,
					},
				},
			},
			NetworkMode:    container.NetworkMode(sandboxConfig.Security.Network),
			ReadonlyRootfs: sandboxConfig.Security.ReadOnly,
			Mounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   dir,
					Target:   sandboxConfig.Mount.WorkDir,
					ReadOnly: sandboxConfig.Mount.ReadOnly,
				},
			},
			CapDrop:     sandboxConfig.Security.CapDrop,
			SecurityOpt: sandboxConfig.Security.SecurityOpt,
		}

		// Create execution context with timeout
		execCtx, cancel := context.WithTimeout(ctx, sandboxConfig.Timeout())
		defer cancel()

		// Create container
		resp, err := cli.ContainerCreate(execCtx, containerConfig, hostConfig, nil, nil, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create container: %v", err)
		}

		// Ensure container cleanup
		defer func() {
			killCtx, killCancel := context.WithTimeout(context.Background(), sandboxConfig.Timeout())
			defer killCancel()

			_ = cli.ContainerRemove(killCtx, resp.ID, container.RemoveOptions{
				Force:         true,
				RemoveVolumes: true,
			})
		}()

		// If there is a "before" step, we need to start the container and then
		// exec the command. This is used for sandboxes that need to start a
		// service before running the command (e.g., a shell).
		if len(sandboxConfig.ExecCommand()) > 0 {
			// Start the container
			if err := cli.ContainerStart(execCtx, resp.ID, container.StartOptions{}); err != nil {
				return nil, fmt.Errorf("failed to start container: %v", err)
			}

			// Wait for container to be running
			if err := waitForContainer(execCtx, cli, resp.ID, 30*time.Second); err != nil {
				return nil, fmt.Errorf("failed to wait for container: %v", err)
			}

			// Create exec instance
			execResp, err := cli.ContainerExecCreate(execCtx, resp.ID, container.ExecOptions{
				Cmd:          sandboxConfig.ExecCommand(),
				AttachStdout: true,
				AttachStderr: true,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create exec: %v", err)
			}

			// Attach to exec
			response, err := cli.ContainerExecAttach(execCtx, execResp.ID, container.ExecAttachOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to attach to exec: %v", err)
			}
			defer response.Close()

			// Read stdout and stderr from the exec command
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			if _, err := stdcopy.StdCopy(stdout, stderr, response.Reader); err != nil {
				return nil, fmt.Errorf("failed to read exec output: %v", err)
			}

			// Wait for the exec command to complete
			for {
				inspectResp, err := cli.ContainerExecInspect(execCtx, execResp.ID)
				if err != nil {
					return nil, fmt.Errorf("failed to inspect exec: %v", err)
				}
				if !inspectResp.Running {
					if inspectResp.ExitCode != 0 {
						if stderr.Len() > 0 {
							return mcp.NewToolResultError(stderr.String()), nil
						}
						return mcp.NewToolResultError(fmt.Sprintf("Command failed with exit code %d", inspectResp.ExitCode)), nil
					}

					if stderr.Len() > 0 {
						stdout.WriteString("\nStderr:\n")
						stdout.Write(stderr.Bytes())
					}

					return mcp.NewToolResultText(stdout.String()), nil
				}
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Wait for execution to finish
		statusCh, errCh := cli.ContainerWait(execCtx, resp.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				return nil, fmt.Errorf("error waiting for container: %v", err)
			}
		case status := <-statusCh:
			logs, err := cli.ContainerLogs(execCtx, resp.ID, container.LogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Timestamps: false,
				Follow:     false,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get logs: %v", err)
			}
			defer logs.Close()

			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			if _, err := stdcopy.StdCopy(stdout, stderr, logs); err != nil {
				return nil, fmt.Errorf("failed to read logs: %v", err)
			}

			if status.StatusCode != 0 {
				return mcp.NewToolResultError(stderr.String()), nil
			}

			if stderr.Len() > 0 {
				stdout.WriteString("\nStderr:\n")
				stdout.Write(stderr.Bytes())
			}

			return mcp.NewToolResultText(stdout.String()), nil
		case <-execCtx.Done():
			return nil, fmt.Errorf("execution timeout after %d seconds", int(sandboxConfig.Timeout().Seconds()))
		}

		return nil, fmt.Errorf("unexpected error: container wait returned no result")
	}
}

// CleanupOrphanedContainers removes any stopped Docker containers from previous runs
func CleanupOrphanedContainers() {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		log.Printf("Failed to create Docker client for cleanup: %v", err)
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List all containers (including stopped) with our label
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(filters.Arg("label", "sandbox-mcp=true")),
	})
	if err != nil {
		log.Printf("Failed to list containers for cleanup: %v", err)
		return
	}

	for _, c := range containers {
		log.Printf("Removing orphaned sandbox container: %s (%s)", c.ID[:12], c.Status)
		_ = cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
	}

	if len(containers) > 0 {
		log.Printf("Cleaned up %d orphaned sandbox containers", len(containers))
	}
}

// CleanupOrphanedSessions removes leftover session directories from previous runs
func CleanupOrphanedSessions() {
	baseDir := DefaultSessionConfig().BaseDir
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to read session base directory for cleanup: %v", err)
		}
		return
	}

	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			path := filepath.Join(baseDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				log.Printf("Failed to remove orphaned session %s: %v", entry.Name(), err)
			} else {
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		log.Printf("Cleaned up %d orphaned session directories", cleaned)
	}
}

// generateSandboxDescription creates a comprehensive description of the sandbox environment
func generateSandboxDescription(sandboxConfig *config.SandboxConfig) string {
	description := sandboxConfig.Description

	if !strings.HasSuffix(description, ".") {
		description += "."
	}
	description += " "

	coreText := "cores"
	if sandboxConfig.Resources.CPU == 1 {
		coreText = "core"
	}

	description += fmt.Sprintf("This sandbox uses the `%s` Docker image, with %d CPU %s, %d MB RAM, and %d processes.",
		sandboxConfig.Image,
		sandboxConfig.Resources.CPU,
		coreText,
		sandboxConfig.Resources.Memory,
		sandboxConfig.Resources.Processes)

	if sandboxConfig.Security.Network == "none" {
		description += " It has no network access"
	} else {
		description += fmt.Sprintf(" It has %s network access", sandboxConfig.Security.Network)
	}

	if sandboxConfig.Mount.ReadOnly || sandboxConfig.Security.ReadOnly {
		description += " and read-only filesystem permissions."
	} else {
		description += " and read-write filesystem permissions."
	}

	if len(sandboxConfig.Parameters.Files) > 0 {
		if len(sandboxConfig.Parameters.Files) == 1 {
			file := sandboxConfig.Parameters.Files[0]
			description += fmt.Sprintf(" It requires a `%s` file", file.Name)
			if file.Description != "" {
				description += fmt.Sprintf(" (%s)", file.Description)
			}
		} else {
			description += " It requires the following files:"
			for i, file := range sandboxConfig.Parameters.Files {
				if i > 0 {
					if i == len(sandboxConfig.Parameters.Files)-1 {
						description += " and"
					} else {
						description += ","
					}
				}
				description += fmt.Sprintf(" `%s`", file.Name)
				if file.Description != "" {
					description += fmt.Sprintf(" (%s)", file.Description)
				}
			}
		}

		if sandboxConfig.Parameters.AdditionalFiles {
			description += " and supports uploading additional files."
		} else {
			description += "."
		}
	} else if sandboxConfig.Parameters.AdditionalFiles {
		description += " It supports uploading additional files."
	}

	description += fmt.Sprintf(" The execution is limited to %d seconds.", sandboxConfig.TimeoutRaw)

	// Add session support information
	description += " Supports session-based persistent storage via `session_id` parameter for iterative development. Use `{filename}_patch` for incremental edits instead of rewriting entire files."

	return description
}
