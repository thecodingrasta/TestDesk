package manager

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/discovery"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type cargoArtifact struct {
	Reason     string `json:"reason"`
	PackageID  string `json:"package_id"`
	Executable string `json:"executable"`
	Success    bool   `json:"success"`
	Target     struct {
		Name string   `json:"name"`
		Kind []string `json:"kind"`
	} `json:"target"`
	Profile struct {
		Test bool `json:"test"`
	} `json:"profile"`
}

var cargoListSummary = regexp.MustCompile(`(?m)^(\d+) tests?, \d+ benchmarks?\r?$`)

// Cargo lists tests separately for every built test executable. Retain the
// package and target, because the same test name can occur in many executables.
func (m *Manager) discoverCargoArtifacts(ctx context.Context, provider domain.Provider, build domain.CommandSpec, stdout string) ([]domain.TestCase, error) {
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	var all []domain.TestCase
	seen := make(map[string]bool)
	finished := false
	for scanner.Scan() {
		var artifact cargoArtifact
		if err := json.Unmarshal(scanner.Bytes(), &artifact); err != nil {
			return nil, fmt.Errorf("invalid Cargo artifact message: %w", err)
		}
		if artifact.Reason == "build-finished" {
			if !artifact.Success {
				return nil, fmt.Errorf("Cargo reported an unsuccessful test build")
			}
			finished = true
		}
		if artifact.Reason != "compiler-artifact" || !artifact.Profile.Test || artifact.Executable == "" {
			continue
		}
		if artifact.PackageID == "" || artifact.Target.Name == "" || len(artifact.Target.Kind) != 1 {
			return nil, fmt.Errorf("Cargo test artifact lacks an unambiguous target identity")
		}
		kind := artifact.Target.Kind[0]
		switch kind {
		case "lib", "bin", "test", "example", "bench":
		default:
			return nil, fmt.Errorf("unsupported Cargo test target kind %q", kind)
		}
		identity := artifact.PackageID + "\x00" + kind + "\x00" + artifact.Target.Name
		if seen[identity] {
			return nil, fmt.Errorf("Cargo produced duplicate test artifact identity %q", artifact.Target.Name)
		}
		seen[identity] = true
		targetArgs, err := runner.CargoTargetArgs(artifact.PackageID, kind, artifact.Target.Name)
		if err != nil {
			return nil, err
		}
		command := build
		command.Args = append([]string{"test"}, targetArgs...)
		command.Args = append(command.Args, "--", "--list")
		listing, err := m.runner.Run(ctx, command, nil)
		if err != nil || listing.ExitCode != 0 || listing.TimedOut || ctx.Err() != nil {
			return nil, fmt.Errorf("list Cargo target %q failed: %v", artifact.Target.Name, err)
		}
		summaries := cargoListSummary.FindAllStringSubmatch(listing.Stdout, -1)
		if len(summaries) != 1 {
			return nil, fmt.Errorf("Cargo target %q did not produce a standard libtest listing", artifact.Target.Name)
		}
		listProvider := provider
		listProvider.Discover.Parser = "cargo-list"
		cases, err := discovery.Parse(listProvider, listing.Stdout)
		if err != nil {
			return nil, err
		}
		expected, err := strconv.Atoi(summaries[0][1])
		if err != nil || len(cases) != expected {
			return nil, fmt.Errorf("Cargo target %q produced an incomplete test listing", artifact.Target.Name)
		}
		for _, tc := range cases {
			hash := sha256.Sum256([]byte(provider.ID + "\x00" + identity + "\x00" + tc.Selector))
			tc.ID = hex.EncodeToString(hash[:8])
			tc.CargoPackage, tc.CargoTargetKind, tc.CargoTargetName = artifact.PackageID, kind, artifact.Target.Name
			packageLabel := artifact.PackageID
			if index := strings.LastIndex(packageLabel, "#"); index >= 0 {
				fragment := packageLabel[index+1:]
				if !strings.Contains(fragment, "@") {
					prefix := strings.TrimRight(packageLabel[:index], "/")
					if slash := strings.LastIndex(prefix, "/"); slash >= 0 {
						fragment = prefix[slash+1:] + "@" + fragment
					}
				}
				packageLabel = fragment
			}
			tc.Module = packageLabel + " / " + kind + " " + artifact.Target.Name
			all = append(all, tc)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !finished {
		return nil, fmt.Errorf("Cargo did not report a complete test build")
	}
	return all, nil
}
