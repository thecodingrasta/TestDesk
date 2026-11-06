package desktop

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestWindowBoundsAllowResizingOnlyOnManager(t *testing.T) {
	ui := domain.UIConfig{ManagerWidth: 1250, ManagerHeight: 825}
	managerBounds := boundsForWindowMode("manager", ui)
	if managerBounds.MaxWidth != 0 || managerBounds.MaxHeight != 0 {
		t.Fatalf("manager must remain resizable: %+v", managerBounds)
	}
	if managerBounds.MinWidth != 920 || managerBounds.MinHeight != 560 {
		t.Fatalf("unexpected manager minimum: %+v", managerBounds)
	}

	for _, mode := range []string{"setup", "settings"} {
		bounds := boundsForWindowMode(mode, ui)
		if bounds.MinWidth != bounds.MaxWidth || bounds.MinHeight != bounds.MaxHeight {
			t.Errorf("%s must use fixed native dimensions: %+v", mode, bounds)
		}
		if bounds.Width != bounds.MinWidth || bounds.Height != bounds.MinHeight {
			t.Errorf("%s initial dimensions must match its fixed bounds: %+v", mode, bounds)
		}
	}
}
