package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseVersionMappings(t *testing.T) {
	cases := []struct {
		release, manifest, bundle string
	}{
		{"1.21a", "1.21.1", "1.21.1"},
		{"1.21b", "1.21.2", "1.21.2"},
		{"1.21z", "1.21.26", "1.21.26"},
		{"1.0.0", "1.0.0", "1.0.0"},
		{"1.0.0-rc.1", "1.0.0-rc.1", "1.0.0"},
	}
	for _, tc := range cases {
		if !validDistributionVersion(tc.release) {
			t.Fatalf("%q should be a valid release version", tc.release)
		}
		manifest, err := manifestVersion(tc.release)
		if err != nil || manifest != tc.manifest {
			t.Fatalf("manifestVersion(%q) = %q, %v; want %q", tc.release, manifest, err, tc.manifest)
		}
		bundle, err := bundleVersion(tc.release)
		if err != nil || bundle != tc.bundle {
			t.Fatalf("bundleVersion(%q) = %q, %v; want %q", tc.release, bundle, err, tc.bundle)
		}
	}
}

func TestMalformedReleaseVersionsAreRejected(t *testing.T) {
	for _, value := range []string{"", "v1.21a", "1.21", "1.21A", "1.21aa", "1.21a1", "01.2.3", "1.21a-rc", "1.2.3-", "1.2.3 ", "1.21a;rm"} {
		if validDistributionVersion(value) {
			t.Errorf("%q must not be accepted as a release version", value)
		}
		if _, err := manifestVersion(value); err == nil {
			t.Errorf("manifestVersion(%q) must fail", value)
		}
		if _, err := bundleVersion(value); err == nil {
			t.Errorf("bundleVersion(%q) must fail", value)
		}
	}
}

func TestPackagedManifestCarriesMappedVersion(t *testing.T) {
	root := repositoryRoot(t)
	def, err := loadDefinition(root)
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	if err = packageDistribution(root, def, def.DefaultVersion, output); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(output, "testdesk_agent_plugin_v"+def.DefaultVersion+".zip")
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, member := range reader.File {
		if member.Name != "plugin.json" {
			continue
		}
		handle, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Version string `json:"version"`
		}
		if err = json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Version != def.ManifestVersion {
			t.Fatalf("packaged plugin.json version = %q; want %q", manifest.Version, def.ManifestVersion)
		}
		return
	}
	t.Fatal("packaged archive omitted plugin.json")
}

func TestWailsProductVersionMustMatchRelease(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(dir, "wails.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"info": {"productVersion": "1.21.2"}}`)
	if err := validateWailsProductVersion(dir, "1.21b"); err != nil {
		t.Fatalf("matching product version rejected: %v", err)
	}
	write(`{"info": {"productVersion": "1.0.0"}}`)
	if err := validateWailsProductVersion(dir, "1.21b"); err == nil {
		t.Fatal("stale product version was accepted")
	}
}
