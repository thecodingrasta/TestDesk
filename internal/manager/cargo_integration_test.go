package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

func TestRealCargoTargetIdentityAndResultEvidence(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_CARGO_TESTS") != "1" {
		t.Skip("requires a configured Rust compiler and linker")
	}
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	for name, contents := range map[string]string{
		"Cargo.toml":     "[package]\nname = \"fixture\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
		"src/lib.rs":     `#[cfg(test)] mod tests { #[test] fn selected() { std::fs::write("lib-ran", "yes").unwrap(); } #[test] fn selected_extra() { std::fs::write("extra-ran", "yes").unwrap(); } #[test] fn failure() { panic!("intentional"); } #[test] #[ignore] fn ignored() {} }`,
		"tests/other.rs": `mod tests { #[test] fn selected() { std::fs::write("integration-ran", "yes").unwrap(); } }`,
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CARGO_NET_OFFLINE", "true")
	m, err := manager.New(root, config.Detect(root), runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var selected []domain.TestCase
	for _, tc := range cases {
		if tc.Selector == "tests::selected" {
			selected = append(selected, tc)
		}
	}
	if len(cases) != 5 || len(selected) != 2 || selected[0].ID == selected[1].ID {
		t.Fatalf("Cargo target identities: %+v", cases)
	}
	if selected[0].Module == selected[1].Module {
		t.Fatalf("Cargo targets must be distinguishable in the catalog: %+v", selected)
	}
	for _, tc := range selected {
		run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tc.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := m.Diagnostics(run.ID, 4096)
		if err != nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 1 {
			t.Fatalf("target %s: %+v %v", tc.CargoTargetKind, diag.Run, err)
		}
		expected := "lib-ran"
		if tc.CargoTargetKind == "test" {
			expected = "integration-ran"
		}
		for _, marker := range []string{"lib-ran", "integration-ran", "extra-ran"} {
			_, err := os.Stat(filepath.Join(root, marker))
			if (err == nil) != (marker == expected) {
				t.Fatalf("target %s marker %s: %v", tc.CargoTargetKind, marker, err)
			}
			if err == nil {
				if err := os.Remove(filepath.Join(root, marker)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for selector, want := range map[string]domain.Status{"tests::failure": domain.StatusFailed, "tests::ignored": domain.StatusSkipped} {
		for _, tc := range cases {
			if tc.Selector != selector {
				continue
			}
			run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tc.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			diag, err := m.Diagnostics(run.ID, 4096)
			if err != nil || diag.Run.Status != want {
				t.Fatalf("%s: %+v %v", selector, diag.Run, err)
			}
		}
	}
	// Retain the discovered ID while removing the underlying case.
	path := filepath.Join(root, "src/lib.rs")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(source), "fn selected()", "fn renamed()", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	var stale domain.TestCase
	for _, tc := range selected {
		if tc.CargoTargetKind == "lib" {
			stale = tc
		}
	}
	run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{stale.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 4096)
	if err != nil || diag.Run.Status != domain.StatusFailed {
		t.Fatalf("zero-match must fail: %+v %v", diag.Run, err)
	}
}

func TestRealCargoWorkspaceTargetsRemainDistinct(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_CARGO_TESTS") != "1" {
		t.Skip("requires a configured Rust compiler and linker")
	}
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	t.Setenv("CARGO_NET_OFFLINE", "true")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[workspace]\nmembers = [\"first\", \"second\"]\nresolver = \"2\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(dir, "src"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \""+name+"\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		code := `#[cfg(test)] mod tests { #[test] fn same() { std::fs::write("` + name + `-ran", "yes").unwrap(); } }`
		if err := os.WriteFile(filepath.Join(dir, "src", "lib.rs"), []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.New(root, config.Detect(root), runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tests, err := m.Discover(ctx)
	if err != nil || len(tests) != 2 {
		t.Fatalf("workspace discovery: %+v %v", tests, err)
	}
	if tests[0].ID == tests[1].ID || tests[0].Module == tests[1].Module {
		t.Fatalf("workspace identities: %+v", tests)
	}
	for _, tc := range tests {
		run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tc.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := m.Diagnostics(run.ID, 4096)
		if err != nil || diag.Run.Status != domain.StatusPassed {
			t.Fatalf("workspace selection: %+v %v", diag.Run, err)
		}
		for _, name := range []string{"first", "second"} {
			path := filepath.Join(root, name, name+"-ran")
			_, err := os.Stat(path)
			expected := tc.CargoTargetName == name
			if (err == nil) != expected {
				t.Fatalf("package %s marker %s: %v", tc.CargoPackage, name, err)
			}
			if err == nil {
				_ = os.Remove(path)
			}
		}
	}
}
