# Security Policy

## Supported Versions

Only the latest release will receive guaranteed security updates until the project reaches version 1.0.0.

## Security Architecture

Sandbox MCP uses Docker containers to provide isolated execution environments with multiple layers of security:

### Container Isolation
- **Capability Dropping**: All sandboxes drop all Linux capabilities (`capDrop: ["all"]`)
- **No New Privileges**: Prevents processes from gaining additional privileges (`securityOpt: ["no-new-privileges:true"]`)
- **Non-root User**: All sandboxes run as a dedicated `sandbox` user, not root
- **Resource Limits**: CPU, memory, process count, and file count restrictions prevent resource exhaustion

### Network Isolation
Most sandboxes use `network: "none"` for complete network isolation. Two exceptions exist:

#### Network-Enabled Sandboxes
- **network-tools**: Uses `bridge` network for network diagnostics (read-only filesystem)
- **apisix**: Uses `bridge` network for API gateway functionality

⚠️ **Warning**: Network-enabled sandboxes can potentially access external networks. Use with caution and only when necessary.

### Filesystem Protection
- **Read-only Mode**: Sandboxes can be configured with read-only filesystems (`readOnly: true`)
- **Temporary Workspaces**: Each execution uses a temporary directory that is cleaned up after completion
- **No Volume Mounts**: User code cannot access host filesystem

## Reporting a Vulnerability

If you find a vulnerability, please report it to [navendu@apache.org](mailto:navendu@apache.org).

Please include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Security Best Practices

### For Users

1. **Review Code Before Execution**: Always review generated code before running it in any sandbox
2. **Prefer Network-Isolated Sandboxes**: Use `network: "none"` sandboxes (python, go, javascript, rust, shell) unless network access is required
3. **Monitor Resource Usage**: Be aware of resource limits and adjust if needed for your use case
4. **Keep Updated**: Always use the latest version of sandbox-mcp for security patches
5. **Understand Bridge Network Risks**: When using `network-tools` or `apisix`, be aware they can access external networks

### For Administrators

1. **Docker Security**: Ensure Docker daemon is properly secured and not exposed to untrusted networks
2. **Resource Limits**: Review and adjust resource limits in sandbox configurations based on your needs
3. **Network Policies**: Consider implementing Docker network policies to restrict bridge network access
4. **Monitoring**: Monitor sandbox execution logs for suspicious activity
5. **Regular Updates**: Keep Docker, sandbox images, and sandbox-mcp binary updated

### For Contributors

1. **Security Review**: All changes to sandbox configurations or execution logic must be security-reviewed
2. **Principle of Least Privilege**: Only grant capabilities and resources that are absolutely necessary
3. **Input Validation**: Validate all user inputs before passing to Docker API
4. **Error Handling**: Ensure errors don't leak sensitive information

## Known Limitations

1. **Docker Socket Access**: Sandbox MCP requires access to Docker socket to create containers. This is a privileged operation that should be protected.
2. **Bridge Network**: Sandboxes with `bridge` network can potentially access external networks and other containers on the same network.
3. **Resource Exhaustion**: While resource limits exist, sophisticated attacks could still cause temporary resource exhaustion.

## Security Audits

The project welcomes security audits and contributions to improve the security posture. Please open an issue or pull request for any security improvements.
