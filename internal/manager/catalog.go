package manager

import (
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// Caller holds m.mu. A transaction merges only these result changes with the
// latest catalog so other managers' completed runs are not overwritten.
func (m *Manager) updateCatalogResultsLocked(changes []domain.TestCase) error {
	tests, err := m.store.UpdateCatalogResults(changes)
	if err != nil {
		for _, change := range changes {
			if current, ok := m.tests[change.ID]; ok {
				current.LastStatus = domain.StatusUnknown
				current.LastFailure = "Result could not be persisted"
				m.tests[change.ID] = current
			}
		}
		return err
	}
	m.setCatalogLocked(tests)
	return nil
}

func (m *Manager) setCatalogLocked(tests []domain.TestCase) {
	m.tests = make(map[string]domain.TestCase, len(tests))
	for _, test := range tests {
		m.tests[test.ID] = test
	}
	modified, err := m.store.CatalogModTime()
	if err == nil {
		m.catalogLoadedAt = modified
	} else {
		m.catalogLoadedAt = time.Time{}
	}
}
