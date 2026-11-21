# TestDesk Skills-Only Plugin

This fallback package bundles the four TestDesk Agent Skills (`testdesk-guide`, `testdesk-running`, `testdesk-debugging`, `testdesk-projects`) without an embedded MCP server configuration. Use it on OpenAI/ChatGPT or Codex directories where local MCP binaries cannot be bundled or organization allowlisting is unavailable, and configure `[mcp_servers.testdesk]` separately in `~/.codex/config.toml` if local MCP access is enabled.
