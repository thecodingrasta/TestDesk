package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const ServerID = "testdesk"

type TargetID string

const (
	TargetUniversal    TargetID = "universal"
	TargetClaude       TargetID = "claude"
	TargetCursorGemini TargetID = "cursor-gemini"
	TargetWindsurf     TargetID = "windsurf"
	TargetCline        TargetID = "cline"
	TargetCodex        TargetID = "codex"
	TargetOpenCode     TargetID = "opencode"
	TargetCLI          TargetID = "cli"

	// Backwards compatibility aliases
	TargetClaudeCode    TargetID = "claude-code"
	TargetClaudeDesktop TargetID = "claude-desktop"
	TargetAntigravity   TargetID = "antigravity"
	TargetCursor        TargetID = "cursor"
)

type Workspace struct {
	ID             TargetID `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Scope          string   `json:"scope"`
	ConfigPath     string   `json:"configPath"`
	Detected       bool     `json:"detected"`
	Installed      bool     `json:"installed"`
	CanInstall     bool     `json:"canInstall"`
	ManualOnly     bool     `json:"manualOnly,omitempty"`
	BackupPath     string   `json:"backupPath,omitempty"`
	CommandPreview []string `json:"commandPreview"`
	Manual         string   `json:"manual"`
	Notes          []string `json:"notes,omitempty"`
}

type Installer struct {
	ProjectRoot string
	BinaryPath  string
}

func NewInstaller(projectRoot string, binaryPath string) (Installer, error) {
	root, err := filepath.Abs(strings.TrimSpace(projectRoot))
	if err != nil {
		return Installer{}, err
	}
	if strings.TrimSpace(binaryPath) == "" {
		binaryPath, err = os.Executable()
		if err != nil {
			return Installer{}, err
		}
	}
	binaryPath, err = filepath.Abs(binaryPath)
	if err != nil {
		return Installer{}, err
	}
	return Installer{ProjectRoot: root, BinaryPath: binaryPath}, nil
}

func (i Installer) Detect() ([]Workspace, error) {
	workspaces := []Workspace{
		i.claudeWorkspace(),
		i.cursorGeminiWorkspace(),
		i.windsurfWorkspace(),
		i.clineWorkspace(),
		i.codexWorkspace(),
		i.openCodeWorkspace(),
		i.universalWorkspace(),
		i.cliWorkspace(),
	}
	for idx := range workspaces {
		workspaces[idx].Manual = i.Manual(workspaces[idx].ID)
		workspaces[idx].CommandPreview = i.command()
		if workspaces[idx].CanInstall {
			workspaces[idx].CanInstall = strings.TrimSpace(i.ProjectRoot) != "" && strings.TrimSpace(i.BinaryPath) != ""
		}
	}
	return workspaces, nil
}

func (i Installer) Install(target TargetID) (Workspace, error) {
	if strings.TrimSpace(i.ProjectRoot) == "" {
		return Workspace{}, errors.New("project root is required")
	}
	if strings.TrimSpace(i.BinaryPath) == "" {
		return Workspace{}, errors.New("testdesk binary path is required")
	}
	backupPath, err := i.backupTarget(target)
	if err != nil {
		return Workspace{}, err
	}
	switch target {
	case TargetCodex:
		err = i.installCodex()
	case TargetClaude, TargetClaudeCode, TargetClaudeDesktop:
		err = i.installClaude()
	case TargetCursorGemini, TargetAntigravity, TargetCursor:
		err = i.installCursorGemini()
	case TargetWindsurf:
		err = i.installWindsurf()
	case TargetCline:
		err = i.installCline()
	case TargetOpenCode:
		err = i.installOpenCode()
	case TargetUniversal, TargetCLI:
		return Workspace{}, errors.New("this target is for manual configuration and copying")
	default:
		err = fmt.Errorf("unknown MCP target %q", target)
	}
	if err != nil {
		return Workspace{}, err
	}
	if strings.TrimSpace(i.ProjectRoot) != "" {
		if _, err := InstallSkills(i.ProjectRoot); err != nil {
			return Workspace{}, fmt.Errorf("MCP configuration was installed, but TestDesk agent instructions could not be installed: %w", err)
		}
	}
	workspaces, detectErr := i.Detect()
	if detectErr != nil {
		return Workspace{}, detectErr
	}
	for _, workspace := range workspaces {
		if workspace.ID == target ||
			(target == TargetClaudeCode && workspace.ID == TargetClaude) ||
			(target == TargetClaudeDesktop && workspace.ID == TargetClaude) ||
			(target == TargetAntigravity && workspace.ID == TargetCursorGemini) ||
			(target == TargetCursor && workspace.ID == TargetCursorGemini) {
			workspace.BackupPath = backupPath
			return workspace, nil
		}
	}
	return Workspace{}, fmt.Errorf("installed %s but could not re-read status", target)
}

func (i Installer) backupTarget(target TargetID) (string, error) {
	path, err := i.targetPath(target)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	backup := path + ".testdesk-backup-" + time.Now().UTC().Format("20060102T150405Z")
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return "", err
	}
	return backup, nil
}

func (i Installer) targetPath(target TargetID) (string, error) {
	switch target {
	case TargetCodex:
		return filepath.Join(homeDir(), ".codex", "config.toml"), nil
	case TargetClaude, TargetClaudeCode:
		return filepath.Join(i.ProjectRoot, ".mcp.json"), nil
	case TargetClaudeDesktop:
		return claudeDesktopConfigPath(), nil
	case TargetCursorGemini, TargetAntigravity:
		return filepath.Join(homeDir(), ".gemini", "config", "mcp_config.json"), nil
	case TargetCursor:
		return filepath.Join(i.ProjectRoot, ".cursor", "mcp.json"), nil
	case TargetWindsurf:
		return filepath.Join(homeDir(), ".codeium", "windsurf", "mcp_config.json"), nil
	case TargetCline:
		return filepath.Join(i.ProjectRoot, ".vscode", "mcp.json"), nil
	case TargetOpenCode:
		return filepath.Join(i.ProjectRoot, "opencode.json"), nil
	case TargetUniversal, TargetCLI:
		return "", nil
	default:
		return "", fmt.Errorf("unknown MCP target %q", target)
	}
}

func (i Installer) Manual(target TargetID) string {
	switch target {
	case TargetCLI:
		if strings.ContainsAny(i.BinaryPath, " \t") {
			return fmt.Sprintf("%q mcp", i.BinaryPath)
		}
		return fmt.Sprintf("%s mcp", i.BinaryPath)
	case TargetCodex:
		return fmt.Sprintf(`[mcp_servers.%s]
command = %q
args = ["mcp"]
enabled = true
startup_timeout_sec = 20
tool_timeout_sec = 600
`, ServerID, i.BinaryPath)
	case TargetOpenCode:
		return mustJSON(map[string]any{"$schema": "https://opencode.ai/config.json", "mcp": map[string]any{ServerID: map[string]any{"type": "local", "command": []string{i.BinaryPath, "mcp"}, "enabled": true, "timeout": 600000}}})
	default:
		return mustJSON(map[string]any{"mcpServers": map[string]any{ServerID: stdioJSON(i.BinaryPath, []string{"mcp"}, "")}})
	}
}

func (i Installer) command() []string {
	return []string{i.BinaryPath, "mcp"}
}

func (i Installer) universalWorkspace() Workspace {
	return Workspace{
		ID:          TargetUniversal,
		Name:        "Universal MCP (JSON stdio)",
		Description: "Universal MCP stdio configuration format compatible with any standard MCP host.",
		Scope:       "Universal format",
		ConfigPath:  "JSON configuration (mcpServers)",
		Detected:    false,
		Installed:   false,
		CanInstall:  false,
		ManualOnly:  true,
		Notes:       []string{"Standard format accepted by Claude, Cursor, Windsurf, Cline, Antigravity and most MCP hosts."},
	}
}

func (i Installer) cliWorkspace() Workspace {
	return Workspace{
		ID:          TargetCLI,
		Name:        "Direct CLI Command",
		Description: "Direct terminal / shell command to run TestDesk in stdio MCP server mode.",
		Scope:       "Shell / Terminal",
		ConfigPath:  "Command line",
		Detected:    false,
		Installed:   false,
		CanInstall:  false,
		ManualOnly:  true,
		Notes:       []string{"Use this command when launching TestDesk as an MCP subprocess from custom scripts or agents."},
	}
}

func (i Installer) claudeWorkspace() Workspace {
	projPath := filepath.Join(i.ProjectRoot, ".mcp.json")
	desktopPath := claudeDesktopConfigPath()
	detected := exists(projPath) || exists(filepath.Join(i.ProjectRoot, "CLAUDE.md")) || exists(desktopPath) || exists(filepath.Dir(desktopPath)) || appExists("claude", "Claude.app")
	installed := jsonHasServer(projPath, "mcpServers") || jsonHasServer(desktopPath, "mcpServers")
	configDisplay := ".mcp.json"
	if exists(desktopPath) {
		configDisplay = ".mcp.json / claude_desktop_config.json"
	}
	return Workspace{
		ID:          TargetClaude,
		Name:        "Claude (Desktop & Code)",
		Description: "Anthropic Claude Code (.mcp.json) & Claude Desktop application configuration.",
		Scope:       "Project & User config",
		ConfigPath:  configDisplay,
		Detected:    detected,
		Installed:   installed,
		CanInstall:  true,
	}
}

func (i Installer) cursorGeminiWorkspace() Workspace {
	geminiPath := filepath.Join(homeDir(), ".gemini", "config", "mcp_config.json")
	legacyPath := filepath.Join(homeDir(), ".gemini", "antigravity", "mcp_config.json")
	cursorPath := filepath.Join(i.ProjectRoot, ".cursor", "mcp.json")
	detected := exists(geminiPath) || exists(legacyPath) || exists(cursorPath) || exists(filepath.Join(i.ProjectRoot, ".cursor")) || appExists("antigravity", "Antigravity.app") || appExists("cursor", "Cursor.app") || commandExists("gemini")
	installed := jsonHasServer(geminiPath, "mcpServers") || jsonHasServer(legacyPath, "mcpServers") || jsonHasServer(cursorPath, "mcpServers")
	return Workspace{
		ID:          TargetCursorGemini,
		Name:        "Antigravity & Cursor",
		Description: "Google Antigravity / Gemini (~/.gemini/config/mcp_config.json) & Cursor IDE (.cursor/mcp.json).",
		Scope:       "Project & User config",
		ConfigPath:  "mcp_config.json / .cursor/mcp.json",
		Detected:    detected,
		Installed:   installed,
		CanInstall:  true,
	}
}

func (i Installer) windsurfWorkspace() Workspace {
	path := filepath.Join(homeDir(), ".codeium", "windsurf", "mcp_config.json")
	return Workspace{
		ID:          TargetWindsurf,
		Name:        "Windsurf / Cascade",
		Description: "Codeium Windsurf IDE & Cascade MCP server configuration.",
		Scope:       "User config",
		ConfigPath:  path,
		Detected:    exists(path) || exists(filepath.Dir(path)) || appExists("windsurf", "Windsurf.app"),
		Installed:   jsonHasServer(path, "mcpServers"),
		CanInstall:  true,
	}
}

func (i Installer) clineWorkspace() Workspace {
	path := filepath.Join(i.ProjectRoot, ".vscode", "mcp.json")
	return Workspace{
		ID:          TargetCline,
		Name:        "Cline / Roo Code / VS Code",
		Description: "VS Code workspace MCP configuration (.vscode/mcp.json) for Cline & Roo Code.",
		Scope:       "Project config",
		ConfigPath:  path,
		Detected:    exists(path) || exists(filepath.Join(i.ProjectRoot, ".vscode")),
		Installed:   jsonHasServer(path, "mcpServers"),
		CanInstall:  true,
	}
}

func (i Installer) codexWorkspace() Workspace {
	path := filepath.Join(homeDir(), ".codex", "config.toml")
	return Workspace{
		ID:          TargetCodex,
		Name:        "Codex",
		Description: "OpenAI Codex CLI / IDE extension stdio MCP configuration.",
		Scope:       "User config",
		ConfigPath:  path,
		Detected:    exists(path) || commandExists("codex"),
		Installed:   fileContains(path, "[mcp_servers."+ServerID+"]"),
		CanInstall:  true,
	}
}

func (i Installer) openCodeWorkspace() Workspace {
	path := filepath.Join(i.ProjectRoot, "opencode.json")
	return Workspace{
		ID:          TargetOpenCode,
		Name:        "OpenCode",
		Description: "Project opencode.json local MCP server configuration.",
		Scope:       "Project config",
		ConfigPath:  path,
		Detected:    exists(path) || exists(filepath.Join(i.ProjectRoot, "AGENTS.md")) || commandExists("opencode"),
		Installed:   jsonHasServer(path, "mcp"),
		CanInstall:  true,
	}
}

func (i Installer) installCodex() error {
	path := filepath.Join(homeDir(), ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content := removeTomlTable(string(data), "mcp_servers."+ServerID)
	block := strings.TrimSpace(i.Manual(TargetCodex)) + "\n"
	if strings.TrimSpace(content) != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n# TestDesk MCP - managed by TestDesk\n" + block
	return os.WriteFile(path, []byte(content), 0o600)
}

func (i Installer) installClaude() error {
	var firstErr error
	if strings.TrimSpace(i.ProjectRoot) != "" {
		path := filepath.Join(i.ProjectRoot, ".mcp.json")
		if err := mergeMCPJSON(path, "mcpServers", stdioJSON(i.BinaryPath, []string{"mcp"}, "")); err != nil {
			firstErr = err
		}
	}
	desktopPath := claudeDesktopConfigPath()
	if exists(desktopPath) || exists(filepath.Dir(desktopPath)) {
		if err := mergeMCPJSON(desktopPath, "mcpServers", stdioJSON(i.BinaryPath, []string{"mcp"}, "")); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (i Installer) installCursorGemini() error {
	var firstErr error
	geminiPath := filepath.Join(homeDir(), ".gemini", "config", "mcp_config.json")
	if err := mergeMCPJSON(geminiPath, "mcpServers", stdioJSON(i.BinaryPath, []string{"mcp"}, "")); err != nil {
		firstErr = err
	}
	if strings.TrimSpace(i.ProjectRoot) != "" {
		cursorPath := filepath.Join(i.ProjectRoot, ".cursor", "mcp.json")
		if exists(filepath.Dir(cursorPath)) || exists(cursorPath) {
			if err := mergeMCPJSON(cursorPath, "mcpServers", stdioJSON(i.BinaryPath, []string{"mcp"}, "")); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (i Installer) installWindsurf() error {
	path := filepath.Join(homeDir(), ".codeium", "windsurf", "mcp_config.json")
	return mergeMCPJSON(path, "mcpServers", stdioJSON(i.BinaryPath, []string{"mcp"}, ""))
}

func (i Installer) installCline() error {
	path := filepath.Join(i.ProjectRoot, ".vscode", "mcp.json")
	return mergeMCPJSON(path, "mcpServers", stdioJSON(i.BinaryPath, []string{"mcp"}, ""))
}

func (i Installer) installOpenCode() error {
	path := filepath.Join(i.ProjectRoot, "opencode.json")
	entry := map[string]any{"type": "local", "command": []string{i.BinaryPath, "mcp"}, "enabled": true, "timeout": 600000}
	return mergeMCPJSON(path, "mcp", entry)
}

func claudeDesktopConfigPath() string {
	home := homeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "Claude", "claude_desktop_config.json")
		}
		return filepath.Join(home, "AppData", "Roaming", "Claude", "claude_desktop_config.json")
	default:
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}
}

func mergeMCPJSON(path string, serverKey string, server map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	root := map[string]any{}
	data, err := os.ReadFile(path)
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			backup := path + ".bak"
			if writeErr := os.WriteFile(backup, data, 0o600); writeErr != nil {
				return fmt.Errorf("%s is not valid JSON and backup failed: %w", path, writeErr)
			}
			root = map[string]any{}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	servers, ok := root[serverKey].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	servers[ServerID] = server
	root[serverKey] = servers
	if _, ok := root["$schema"]; !ok && strings.EqualFold(filepath.Base(path), "opencode.json") {
		root["$schema"] = "https://opencode.ai/config.json"
	}
	data, err = json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func stdioJSON(command string, args []string, cwd string) map[string]any {
	result := map[string]any{
		"command": command,
	}
	if len(args) > 0 {
		result["args"] = args
	}
	if strings.TrimSpace(cwd) != "" {
		result["cwd"] = cwd
	}
	return result
}

func jsonHasServer(path string, key string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return false
	}
	servers, ok := root[key].(map[string]any)
	if !ok {
		return false
	}
	_, ok = servers[ServerID]
	return ok
}

func removeTomlTable(content string, table string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	target := "[" + table + "]"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == target {
			skip = true
			continue
		}
		if skip && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			skip = false
		}
		if !skip {
			out = append(out, line)
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

func quoteList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}

func mustJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileContains(path string, needle string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), needle)
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func appExists(name string, macBundleName string) bool {
	if commandExists(name) {
		return true
	}
	if runtime.GOOS == "darwin" && macBundleName != "" {
		if exists(filepath.Join("/Applications", macBundleName)) ||
			exists(filepath.Join(homeDir(), "Applications", macBundleName)) {
			return true
		}
	}
	return false
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		return home
	}
	return "."
}

func Sort(workspaces []Workspace) {
	sort.SliceStable(workspaces, func(i int, j int) bool {
		return workspaces[i].Name < workspaces[j].Name
	})
}
