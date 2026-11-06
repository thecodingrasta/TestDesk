package desktop

import "github.com/thecodingrasta/testdesk/internal/domain"

func shouldDisplayWidget(mode domain.VisibilityMode, active bool, manual bool) bool {
	switch mode {
	case domain.VisibilityAlways:
		return true
	case domain.VisibilityWhenActive, "":
		return active || manual
	default:
		return false
	}
}

func shouldKeepWidgetOnTop(ui domain.UIConfig, active bool) bool {
	switch ui.TopmostMode {
	case domain.VisibilityAlways:
		return true
	case domain.VisibilityWhenActive:
		return active
	default:
		return false
	}
}
