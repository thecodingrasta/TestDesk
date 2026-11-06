package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/thecodingrasta/testdesk/internal/agents"
	"github.com/thecodingrasta/testdesk/internal/desktop"
	"github.com/thecodingrasta/testdesk/internal/mcpbridge"
)

const desktopVersion = "1.0.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-version":
			fmt.Println(desktopVersion)
			return
		case "mcp":
			if err := runMCP(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "testdesk mcp:", err)
				os.Exit(1)
			}
			return
		case "mcp-targets":
			if err := printMCPTargets(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "testdesk mcp-targets:", err)
				os.Exit(1)
			}
			return
		case "mcp-install":
			if err := installMCP(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "testdesk mcp-install:", err)
				os.Exit(1)
			}
			return
		}
	}

	project := flag.String("project", "", "project root; when omitted TestDesk reopens the last project or starts the first-run setup")
	agentStart := flag.Bool("agent-start", false, "start in the tray for an MCP client")
	flag.Parse()
	if err := desktop.Run(*project, *agentStart); err != nil {
		fmt.Fprintln(os.Stderr, "testdesk desktop:", err)
		os.Exit(1)
	}
}

func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	project := fs.String("project", "", "project root (optional; when omitted, TestDesk serves all registered projects)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return mcpbridge.Serve(ctx, os.Stdin, os.Stdout, *project, executable)
}

func printMCPTargets(args []string) error {
	fs := flag.NewFlagSet("mcp-targets", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	installer, err := agents.NewInstaller(*project, "")
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

func installMCP(args []string) error {
	fs := flag.NewFlagSet("mcp-install", flag.ExitOnError)
	project := fs.String("project", ".", "project root")
	target := fs.String("target", "", "codex|claude-code|opencode|antigravity")
	if err := fs.Parse(args); err != nil {
		return err
	}
	installer, err := agents.NewInstaller(*project, "")
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
