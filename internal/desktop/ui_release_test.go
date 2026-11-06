package desktop

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestReleaseUIAssetsPreserveRequiredV1Flows(t *testing.T) {
	html := embeddedText(t, "assets/index.html")
	javascript := embeddedText(t, "assets/app.js")
	stylesheet := embeddedText(t, "assets/style.css")

	requiredHTML := []string{
		`href="./testdesk-brand-icon.png"`,
		`src="./testdesk-brand-horizontal.png"`,
		`id="openAddProjectBtn"`,
		`id="addProjectSheet"`,
		`id="activeRunTabBtn"`,
		`id="recentRunsTabBtn"`,
		`id="catalogLoader"`,
		`<span>Discovered Tests</span>`,
		`<span>Last Result</span>`,
		`<option value="">All Providers</option>`,
		`<option value="">All Layers</option>`,
		`<option value="">All Modules</option>`,
		`<option value="">Any Status</option>`,
		`id="widgetModeSelect"`,
		`id="topmostModeSelect"`,
		`id="logRetentionSelect"`,
		`id="resetWidgetBtn"`,
		`id="mcpSummary"`,
		`id="generalSettingsTabBtn"`,
		`id="agentSettingsTabBtn"`,
		`id="widgetStateLabel"`,
		`id="widgetActionBtn"`,
		`pause-glyph`,
		`data-tooltip="Open Manager"`,
	}
	for _, fragment := range requiredHTML {
		if !strings.Contains(html, fragment) {
			t.Errorf("index.html is missing required release UI fragment %q", fragment)
		}
	}
	for _, removed := range []string{`class="project-context"`, `id="activeProjectPath"`, `testdesk-icon.svg`} {
		if strings.Contains(html, removed) {
			t.Errorf("index.html still contains removed temporary UI fragment %q", removed)
		}
	}

	requiredJavaScript := []string{
		"scheduleFilterRender",
		"filterCatalogue(catalogueTests)",
		"CATALOGUE_BATCH_SIZE",
		"data-show-all-tests",
		"visibleTestLimit = filteredTests.length",
		"formatStatus(snapshot?.paused ? 'paused'",
		"cancelled: 'Stopped'",
		"setCompactState(active ? (widgetPaused ? 'Paused' : 'Queued') : 'Idle', 'state-warning')",
		"compactStateTone",
		"setRunPane('active')",
		"setSettingsPane('general')",
		"setAddProjectSheet(true)",
		"ResetWidgetPosition",
		"mode === 'manager' && includeTests",
		"setInterval(() => refresh(false), 1200)",
		"ensureManagerBounds",
	}
	for _, fragment := range requiredJavaScript {
		if !strings.Contains(javascript, fragment) {
			t.Errorf("app.js is missing required release behaviour %q", fragment)
		}
	}
	if strings.Contains(javascript, "debounce(() => refresh(true), 180)") {
		t.Fatal("filter inputs still trigger the slow server catalogue refresh path")
	}

	requiredCSS := []string{
		".page-scroll",
		".settings-content",
		".settings-pane",
		".splash-wordmark",
		".add-project-sheet",
		".run-tab-pane .log-output",
		".run-tab-pane .recent",
		".catalog-loader",
		"--wails-draggable:drag",
		".compact-open #app.compact .player",
		".widget-state.state-ready",
		".widget-state.state-warning",
		".widget-state.state-danger",
		".player .widget-close",
	}
	for _, fragment := range requiredCSS {
		if !strings.Contains(stylesheet, fragment) {
			t.Errorf("style.css is missing required release layout %q", fragment)
		}
	}
}

func TestReleaseBrandAssetsAreEmbeddedAndDecodable(t *testing.T) {
	for _, path := range []string{"assets/testdesk-brand-icon.png", "assets/testdesk-brand-horizontal.png"} {
		data, err := assets.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(data) < 16_000 {
			t.Fatalf("%s is unexpectedly small: %d bytes", path, len(data))
		}
		if _, err := png.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
}

func embeddedText(t *testing.T, path string) string {
	t.Helper()
	data, err := assets.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
