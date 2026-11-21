package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cur := wd
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			t.Fatalf("could not locate go.mod above %s", wd)
		}
		cur = parent
	}
}

func TestRepositoryDistributionValidates(t *testing.T) {
	root := repositoryRoot(t)
	if err := validateDistribution(root); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReleaseVersionMatchesGitSource(t *testing.T) {
	root := repositoryRoot(t)
	t.Chdir(root)
	def, err := loadDefinition(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "--release-version", def.DefaultVersion + "-mismatch"}); err == nil || !strings.Contains(err.Error(), "does not match Git-source plugin version") {
		t.Fatalf("mismatched release was accepted: %v", err)
	}
	if err := run([]string{"validate", "--release-version", def.DefaultVersion}); err != nil {
		t.Fatal(err)
	}
}

func TestPackagesAreReproducibleCompleteAndChecksummed(t *testing.T) {
	root := repositoryRoot(t)
	def, err := loadDefinition(root)
	if err != nil {
		t.Fatal(err)
	}
	first := t.TempDir()
	second := t.TempDir()
	if err = packageDistribution(root, def, def.DefaultVersion, first); err != nil {
		t.Fatal(err)
	}
	if err = packageDistribution(root, def, def.DefaultVersion, second); err != nil {
		t.Fatal(err)
	}
	checksums, err := os.ReadFile(filepath.Join(first, "AGENT_PLUGIN_SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(checksums), "\n") {
		t.Fatal("checksum list must end with a newline")
	}
	lines := strings.Split(strings.TrimSpace(string(checksums)), "\n")
	if len(lines) != len(def.Targets) {
		t.Fatalf("checksummed package count = %d, want %d", len(lines), len(def.Targets))
	}
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			t.Fatalf("invalid checksum row %q", line)
		}
		firstBody, err := os.ReadFile(filepath.Join(first, parts[1]))
		if err != nil {
			t.Fatal(err)
		}
		secondBody, err := os.ReadFile(filepath.Join(second, parts[1]))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(firstBody)
		if hex.EncodeToString(digest[:]) != parts[0] || string(firstBody) != string(secondBody) {
			t.Fatalf("package %s checksum or reproducibility mismatch", parts[1])
		}
	}
	for _, item := range def.Targets {
		filename := item.Archive + "_v" + def.DefaultVersion + ".zip"
		reader, err := zip.OpenReader(filepath.Join(first, filename))
		if err != nil {
			t.Fatal(err)
		}
		members := map[string]*zip.File{}
		for _, member := range reader.File {
			members[member.Name] = member
		}
		for _, required := range []string{"README.md", "LICENSE", "MARKETPLACE-METADATA.json"} {
			if _, found := members[required]; !found {
				t.Errorf("%s omitted %s", item.ID, required)
			}
		}
		if item.ID == "antigravity" {
			if _, found := members["assets/logo.png"]; !found {
				t.Errorf("antigravity archive omitted assets/logo.png")
			}
		}
		_ = reader.Close()
	}
}
