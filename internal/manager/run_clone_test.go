package manager

import (
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestCloneRunCopiesNestedMutableFields(t *testing.T) {
	started := time.Unix(10, 0)
	finished := time.Unix(20, 0)
	original := domain.Run{Items: []domain.RunItem{{Warnings: []string{"original"}, StartedAt: &started, FinishedAt: &finished}}}
	cloned := cloneRun(original)
	cloned.Items[0].Warnings[0] = "changed"
	*cloned.Items[0].StartedAt = time.Unix(30, 0)
	*cloned.Items[0].FinishedAt = time.Unix(40, 0)
	if original.Items[0].Warnings[0] != "original" || !started.Equal(time.Unix(10, 0)) || !finished.Equal(time.Unix(20, 0)) {
		t.Fatalf("clone retained mutable references: %+v", original.Items[0])
	}
}
