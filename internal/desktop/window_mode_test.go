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
		if bounds.Width > bounds.MaxWidth || bounds.Height > bounds.MaxHeight || bounds.MaxWidth <= 0 || bounds.MaxHeight <= 0 {
			t.Errorf("%s must cap maximum native dimensions: %+v", mode, bounds)
		}
		if bounds.MinWidth > bounds.Width || bounds.MinHeight > bounds.Height || bounds.MinWidth <= 0 || bounds.MinHeight <= 0 {
			t.Errorf("%s minimum dimensions must allow shrinking below initial size: %+v", mode, bounds)
		}
	}
}
