package desktop

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestShouldDisplayWidget(t *testing.T) {
	tests := []struct {
		name   string
		mode   domain.VisibilityMode
		active bool
		manual bool
		want   bool
	}{
		{name: "always idle", mode: domain.VisibilityAlways, want: true},
		{name: "active while running", mode: domain.VisibilityWhenActive, active: true, want: true},
		{name: "active manual compact", mode: domain.VisibilityWhenActive, manual: true, want: true},
		{name: "active idle automatic", mode: domain.VisibilityWhenActive, want: false},
		{name: "off while active", mode: domain.VisibilityOff, active: true, manual: true, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldDisplayWidget(test.mode, test.active, test.manual); got != test.want {
				t.Fatalf("shouldDisplayWidget(%q, %t, %t) = %t, want %t", test.mode, test.active, test.manual, got, test.want)
			}
		})
	}
}

func TestShouldKeepWidgetOnTop(t *testing.T) {
	tests := []struct {
		name   string
		mode   domain.VisibilityMode
		active bool
		want   bool
	}{
		{name: "always idle", mode: domain.VisibilityAlways, want: true},
		{name: "active while running", mode: domain.VisibilityWhenActive, active: true, want: true},
		{name: "active while idle", mode: domain.VisibilityWhenActive, want: false},
		{name: "off while running", mode: domain.VisibilityOff, active: true, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ui := domain.UIConfig{TopmostMode: test.mode}
			if got := shouldKeepWidgetOnTop(ui, test.active); got != test.want {
				t.Fatalf("shouldKeepWidgetOnTop(%q, %t) = %t, want %t", test.mode, test.active, got, test.want)
			}
		})
	}
}

func TestWidgetVisibilityTargetShowsEnabledWidgetWhenRunStarts(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		visibility domain.VisibilityMode
		active     bool
		manual     bool
		want       string
	}{
		{name: "always preserves manager", current: "manager", visibility: domain.VisibilityAlways, want: ""},
		{name: "always reopens from hidden", current: "manager_hidden", visibility: domain.VisibilityAlways, want: "compact_auto"},
		{name: "when active reopens from hidden", current: "manager_hidden", visibility: domain.VisibilityWhenActive, active: true, want: "compact_auto"},
		{name: "when active stays in manager while active", current: "manager", visibility: domain.VisibilityWhenActive, active: true, want: ""},
		{name: "when active stays in manager while idle", current: "manager", visibility: domain.VisibilityWhenActive, want: ""},
		{name: "automatic compact hides after run", current: "compact", visibility: domain.VisibilityWhenActive, want: "manager_hidden"},
		{name: "off hides automatic compact", current: "compact", visibility: domain.VisibilityOff, active: true, manual: false, want: "manager_hidden"},
		{name: "off allows manual compact to stay open", current: "compact", visibility: domain.VisibilityOff, active: true, manual: true, want: ""},
		{name: "off does not disturb manager", current: "manager", visibility: domain.VisibilityOff, active: true, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := widgetVisibilityTarget(test.current, test.visibility, test.active, test.manual)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("widgetVisibilityTarget(%q, %q, %t, %t) = %q, want %q", test.current, test.visibility, test.active, test.manual, got, test.want)
			}
		})
	}
}

func TestSafeUIKeepsCompactWindowBelowManagerDimensions(t *testing.T) {
	ui := safeUI(domain.UIConfig{CompactWidth: 3000, CompactHeight: 3000, CompactExpandedHeight: 3000, ManagerWidth: 1250, ManagerHeight: 825})
	if ui.CompactWidth >= ui.ManagerWidth || ui.CompactHeight >= ui.ManagerHeight {
		t.Fatalf("compact dimensions must never reach manager dimensions: compact=%dx%d manager=%dx%d", ui.CompactWidth, ui.CompactHeight, ui.ManagerWidth, ui.ManagerHeight)
	}
}

func TestResetWidgetPositionClearsPersistedCoordinates(t *testing.T) {
	root := t.TempDir()
	cfg := config.Normalise(root, domain.ProjectConfig{})
	cfg.UI.CompactX = 1420
	cfg.UI.CompactY = 20
	cfg.UI.CompactPositionSet = true
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	mgr, err := manager.NewFromProject(root)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{
		projectRoot:       root,
		managers:          map[string]*manager.Manager{root: mgr},
		windowMode:        "settings",
		compactPositioned: true,
	}
	if err := app.ResetWidgetPosition(); err != nil {
		t.Fatal(err)
	}
	updated := mgr.Config()
	if updated.UI.CompactPositionSet || updated.UI.CompactX != 0 || updated.UI.CompactY != 0 {
		t.Fatalf("compact position was not reset: %+v", updated.UI)
	}
	if app.compactPositioned {
		t.Fatal("reset must allow the next compact open to apply the bottom-left default")
	}
}

func TestSaveUIUpdatesWidgetSizeAndMode(t *testing.T) {
	root := t.TempDir()
	cfg := config.Normalise(root, domain.ProjectConfig{})
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	mgr, err := manager.NewFromProject(root)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{
		projectRoot: root,
		managers:    map[string]*manager.Manager{root: mgr},
	}
	ui, err := app.SaveUI("always", "off", 14, "small")
	if err != nil {
		t.Fatal(err)
	}
	if ui.WidgetSize != "small" || ui.WidgetMode != domain.VisibilityAlways || ui.LogRetentionDays != 14 {
		t.Fatalf("unexpected UI config returned: %+v", ui)
	}
	if ui.CompactWidth != 380 || ui.CompactHeight != 96 {
		t.Fatalf("expected small compact bounds, got %dx%d", ui.CompactWidth, ui.CompactHeight)
	}
}
