// Command agentdist validates, synchronizes, and packages reusable Agent Plugins and native adapters.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: go run ./tools/agentdist validate [--config PATH] [--release-version VERSION] | sync-skills [--config PATH] [--overwrite] | package [--config PATH] [--version VERSION] [--output DIRECTORY]")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	module, err := os.ReadFile(filepath.Join(root, "go.mod"))
	moduleLine := ""
	if err == nil {
		moduleLine = strings.TrimSuffix(strings.SplitN(string(module), "\n", 2)[0], "\r")
	}
	moduleFields := strings.Fields(moduleLine)
	if err != nil || len(moduleFields) != 2 || moduleFields[0] != "module" {
		return fmt.Errorf("run this command from a Go module source root")
	}
	switch arguments[0] {
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		config := flags.String("config", defaultDefinitionPath, "product distribution definition relative to the repository root")
		releaseVersion := flags.String("release-version", "", "require the Git-source plugin version to match this release version without a leading v")
		if err = flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected validate arguments: %s", strings.Join(flags.Args(), " "))
		}
		if *releaseVersion != "" {
			def, err := loadDefinitionAt(root, *config)
			if err != nil {
				return err
			}
			if def.DefaultVersion != *releaseVersion {
				return fmt.Errorf("release version %q does not match Git-source plugin version %q", *releaseVersion, def.DefaultVersion)
			}
		}
		if err = validateDistributionAt(root, *config); err != nil {
			return err
		}
		fmt.Println("Agent distribution metadata, skills, MCP contracts, adapters, and package inputs are valid.")
		return nil
	case "sync-skills":
		flags := flag.NewFlagSet("sync-skills", flag.ContinueOnError)
		config := flags.String("config", defaultDefinitionPath, "product distribution definition relative to the repository root")
		overwrite := flags.Bool("overwrite", false, "refresh stale generated skills and license notices in publication sources")
		if err = flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected sync-skills arguments: %s", strings.Join(flags.Args(), " "))
		}
		def, err := loadDefinitionAt(root, *config)
		if err != nil {
			return err
		}
		if err = syncPublishedSkills(root, def, *overwrite); err != nil {
			return err
		}
		fmt.Println("Synchronized canonical skills and license notices to declared repository publication sources.")
		return nil
	case "package":
		flags := flag.NewFlagSet("package", flag.ContinueOnError)
		config := flags.String("config", defaultDefinitionPath, "product distribution definition relative to the repository root")
		version := flags.String("version", "", "release version without a leading v (defaults to distribution metadata)")
		output := flags.String("output", filepath.Join("dist"), "output directory for deterministic ZIP packages and checksums")
		if err = flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected package arguments: %s", strings.Join(flags.Args(), " "))
		}
		def, err := loadDefinitionAt(root, *config)
		if err != nil {
			return err
		}
		if *version == "" {
			*version = def.DefaultVersion
		}
		if err = validateDistributionAt(root, *config); err != nil {
			return err
		}
		if !validDistributionVersion(*version) {
			return fmt.Errorf("invalid distribution version %q; expected semantic version without a leading v", *version)
		}
		directory := *output
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(root, directory)
		}
		if err = packageDistribution(root, def, *version, directory); err != nil {
			return err
		}
		fmt.Printf("Generated %d Agent Plugin packages at %s\n", len(def.Targets), directory)
		return nil
	default:
		return fmt.Errorf("unknown task %q; expected validate, sync-skills, or package", arguments[0])
	}
}
