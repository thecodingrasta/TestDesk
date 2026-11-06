package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestRealJestAmbiguousProjectsDoNotExecuteSelectedFile(t *testing.T) {
	install := os.Getenv("TESTDESK_JEST_INSTALL")
	if install == "" {
		t.Skip("set TESTDESK_JEST_INSTALL to a disposable Jest installation")
	}
	root, err := os.MkdirTemp(install, "multiproject-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	t.Setenv("npm_config_cache", filepath.Join(root, ".npm-cache"))
	for name, content := range map[string]string{
		"package.json":   `{"private":true}`,
		"jest.config.js": `module.exports={maxWorkers:2,projects:[{displayName:'A',rootDir:__dirname,testMatch:['**/same.test.js']},{displayName:'A',rootDir:__dirname,testMatch:['**/same.test.js']}]};`,
		"same.test.js":   `test('same', () => require('fs').appendFileSync('marker', 'ran'));`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	provider := config.JestProvider()
	m, err := manager.New(root, domain.ProjectConfig{Version: 1, Name: "jest", Providers: []domain.Provider{provider}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tests, err := m.Discover(ctx)
	if err == nil || len(tests) != 0 {
		t.Fatalf("discovery: %+v, %v", tests, err)
	}
	if _, err := os.Stat(filepath.Join(root, "marker")); !os.IsNotExist(err) {
		t.Fatalf("ambiguous test executed: %v", err)
	}
}
