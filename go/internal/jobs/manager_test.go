package jobs

import (
	"testing"

	"github.com/garnizeh/eth-scanner/internal/database"
)

func TestNewManager(t *testing.T) {
	var q *database.Queries
	m := New(q)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if m.db != q {
		t.Fatalf("expected db to be %v, got %v", q, m.db)
	}
}
