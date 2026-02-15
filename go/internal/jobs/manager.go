package jobs

import "github.com/garnizeh/eth-scanner/internal/database"

// Manager encapsulates job management operations.
type Manager struct {
	db *database.Queries
}

// New constructs a new Manager with the provided database queries.
func New(db *database.Queries) *Manager {
	return &Manager{db: db}
}
