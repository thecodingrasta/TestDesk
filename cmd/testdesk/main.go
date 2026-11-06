package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/thecodingrasta/testdesk/internal/agents"
	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
	"github.com/thecodingrasta/testdesk/internal/mcpbridge"
)

const version = "1.0.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "testdesk:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "version":
		fmt.Println(version)
		return nil
	case "init":
		return initCmd(args[1:])
	case "doctor":
		return doctorCmd(args[1:])
	case "discover":
		return discoverCmd(args[1:])
	case "list":
		return listCmd(args[1:])
	case "run":
		return runCmd(args[1:])
	case "status":
		return statusCmd(args[1:])
	case "control":
		return controlCmd(args[1:])
	case "diagnostics":
		return diagnosticsCmd(args[1:])
	case "mcp":
		return mcpCmd(args[1:])
	case "mcp-targets":
		return mcpTargetsCmd(args[1:])
	case "mcp-install":
		return mcpInstallCmd(args[1:])
	case "mcp-smoke":
		return mcpSmokeCmd(args[1:])
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func initCmd(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	force := fs.Bool("force", false, "overwrite existing config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := abs(*project)
	if err != nil {
		return err
	}
	cfg, err := config.Init(root, *force)
	if err != nil {
		return err
	}
	fmt.Printf("created %s with %d provider(s)\n", filepath.Join(root, config.FileName), len(cfg.Providers))
	return nil
}

func doctorCmd(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := abs(*project)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	mgr, err := manager.New(root, cfg, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Project: %s\nWorkspace: %s\nProviders: %d\n", cfg.Name, mgr.StoreRoot(), len(cfg.Providers))
	for _, provider := range cfg.Providers {
		state := "disabled"
		if provider.Enabled {
			state = "enabled"
		}
		fmt.Printf("- %s [%s/%s] %s\n", provider.ID, provider.Kind, provider.Layer, state)
	}
	return nil
}

func discoverCmd(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgr, err := loadManager(*project)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	tests, discoverErr := mgr.Discover(ctx)
	if *jsonOut {
		return printJSON(map[string]any{"count": len(tests), "tests": tests, "warning": errorString(discoverErr)})
	}
	fmt.Printf("discovered %d test(s)\n", len(tests))
	if discoverErr != nil {
		fmt.Println("warning:", discoverErr)
	}
	return nil
}

func listCmd(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	provider := fs.String("provider", "", "provider id")
	layer := fs.String("layer", "", "layer filter")
	status := fs.String("status", "", "last status filter")
	query := fs.String("query", "", "text search")
	limit := fs.Int("limit", 200, "max tests")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgr, err := loadManager(*project)
	if err != nil {
		return err
	}
	filter := domain.Filter{ProviderIDs: splitCSV(*provider), Layers: splitCSV(*layer), Statuses: splitStatuses(*status), Query: *query, Limit: *limit}
	snapshot, err := mgr.Snapshot(filter)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(snapshot.Tests)
	}
	for _, test := range snapshot.Tests {
		fmt.Printf("%s %-11s %-10s %-9s %s\n", test.ID, test.ProviderID, test.Layer, test.LastStatus, test.FullName)
	}
	return nil
}

func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	testIDs := fs.String("tests", "", "comma separated test ids")
	providers := fs.String("providers", "", "comma separated provider ids")
	query := fs.String("query", "", "run tests matching query")
	wait := fs.Bool("wait", true, "wait until run completes")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgr, err := loadManager(*project)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	run, err := mgr.Run(ctx, manager.RunRequest{TestIDs: splitCSV(*testIDs), ProviderIDs: splitCSV(*providers), Filter: domain.Filter{Query: *query}})
	if err != nil {
		return err
	}
	if *wait {
		if err := mgr.WaitForIdle(ctx); err != nil {
			return err
		}
		diag, _ := mgr.Diagnostics(run.ID, 4096)
		if diag.Run != nil {
			run = *diag.Run
		}
	}
	if *jsonOut {
		if err := printJSON(run); err != nil {
			return err
		}
		if *wait && (run.Status == domain.StatusFailed || run.Failed > 0) {
			return errors.New("test run completed with failures")
		}
		return nil
	}
	fmt.Printf("run %s queued with %d item(s)\n", run.ID, run.Total)
	if *wait {
		fmt.Printf("status: %s passed=%d failed=%d skipped=%d cancelled=%d\n", run.Status, run.Passed, run.Failed, run.Skipped, run.Cancelled)
		if run.Status == domain.StatusFailed || run.Failed > 0 {
			return errors.New("test run completed with failures")
		}
	}
	return nil
}

func statusCmd(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	includeTests := fs.Bool("tests", false, "include test page")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgr, err := loadManager(*project)
	if err != nil {
		return err
	}
	snapshot, err := mgr.Snapshot(domain.Filter{Limit: 200})
	if err != nil {
		return err
	}
	if *jsonOut {
		if !*includeTests {
			snapshot.Tests = nil
		}
		return printJSON(snapshot)
	}
	fmt.Printf("%s: %d tests, %d provider(s)\n", snapshot.ProjectName, snapshot.TestCount, snapshot.ProviderCount)
	return nil
}

func controlCmd(args []string) error {
	fs := flag.NewFlagSet("control", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	action := fs.String("action", "", "stop|kill|skip|pause|resume")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgr, err := loadManager(*project)
	if err != nil {
		return err
	}
	if err := mgr.Control(manager.ControlAction(*action)); err != nil {
		return err
	}
	fmt.Printf("applied control action %s\n", *action)
	return nil
}

func diagnosticsCmd(args []string) error {
	fs := flag.NewFlagSet("diagnostics", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	runID := fs.String("run", "", "run id (defaults to active run)")
	maxBytes := fs.Int64("max-bytes", 4096, "maximum log bytes to return")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgr, err := loadManager(*project)
	if err != nil {
		return err
	}
	diag, err := mgr.Diagnostics(*runID, *maxBytes)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(diag)
	}
	fmt.Println(diag.Log)
	return nil
}

func mcpCmd(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	project := fs.String("project", "", "project root (optional; when omitted, TestDesk serves all registered projects)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return mcpbridge.Serve(ctx, os.Stdin, os.Stdout, *project, mcpbridge.DesktopExecutableForCLI())
}

func mcpTargetsCmd(args []string) error {
	fs := flag.NewFlagSet("mcp-targets", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := abs(*project)
	if err != nil {
		return err
	}
	installer, err := agents.NewInstaller(root, "")
	if err != nil {
		return err
	}
	workspaces, err := installer.Detect()
	if err != nil {
		return err
	}
	for _, workspace := range workspaces {
		state := "available"
		if workspace.Installed {
			state = "installed"
		} else if !workspace.Detected {
			state = "not detected"
		}
		fmt.Printf("%s\t%s\t%s\n", workspace.ID, state, workspace.ConfigPath)
	}
	return nil
}

func mcpInstallCmd(args []string) error {
	fs := flag.NewFlagSet("mcp-install", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	target := fs.String("target", "", "codex|claude-code|opencode|antigravity")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := abs(*project)
	if err != nil {
		return err
	}
	installer, err := agents.NewInstaller(root, "")
	if err != nil {
		return err
	}
	workspace, err := installer.Install(agents.TargetID(*target))
	if err != nil {
		return err
	}
	fmt.Printf("installed %s MCP config at %s\n", workspace.Name, workspace.ConfigPath)
	return nil
}

func mcpSmokeCmd(args []string) error {
	fs := flag.NewFlagSet("mcp-smoke", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mgr, err := loadManager(*project)
	if err != nil {
		return err
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{"includeTests":false}}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := mcp.New(mgr).Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		return err
	}
	if !strings.Contains(output.String(), "testdesk_snapshot") || !strings.Contains(output.String(), "project") {
		return errors.New("MCP smoke test did not return expected payload")
	}
	fmt.Print(output.String())
	return nil
}

func loadManager(project string) (*manager.Manager, error) {
	root, err := abs(project)
	if err != nil {
		return nil, err
	}
	return manager.NewFromProject(root)
}

func abs(path string) (string, error) {
	if path == "" {
		path = "."
	}
	return filepath.Abs(path)
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitStatuses(value string) []domain.Status {
	parts := splitCSV(value)
	statuses := make([]domain.Status, 0, len(parts))
	for _, part := range parts {
		statuses = append(statuses, domain.Status(part))
	}
	return statuses
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func usage() {
	fmt.Print(`TestDesk ` + version + `

Usage:
  testdesk init --project <path>
  testdesk discover --project <path>
  testdesk list --project <path> [--provider vitest] [--query login]
  testdesk run --project <path> [--tests id1,id2 | --providers playwright,vitest | --query login]
  testdesk status --project <path>
  testdesk control --project <path> --action pause|resume|stop|kill|skip
  testdesk diagnostics --project <path> --run <run-id>
  testdesk mcp --project <path>
  testdesk mcp-targets --project <path>
  testdesk mcp-install --project <path> --target codex|claude-code|opencode|antigravity

Desktop build:
  Windows: .\scripts\windows\TestDesk.bat
  Linux/macOS: bash scripts/build.sh
`)
}
