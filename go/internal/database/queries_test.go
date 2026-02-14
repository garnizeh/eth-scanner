package database

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*sql.DB, *Queries) {
	ctx := context.Background()
	dbPath := "test_queries.db"
	t.Cleanup(func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove db file: %v", err)
		}
	})

	db, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}

	return db, NewQueries(db)
}

func TestCreateAndLeaseBatch(t *testing.T) {
	ctx := context.Background()
	db, queries := setupTestDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close failed: %v", err)
		}
	}()

	// Create a batch
	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28}

	job, err := queries.CreateBatch(ctx, CreateBatchParams{
		Prefix28:           prefix,
		NonceStart:         0,
		NonceEnd:           1000000,
		CurrentNonce:       sql.NullInt64{Valid: false},
		WorkerID:           sql.NullString{String: "test-worker-1", Valid: true},
		WorkerType:         sql.NullString{String: "pc", Valid: true},
		Column7:            sql.NullString{String: "3600", Valid: true}, // expires in 1 hour
		RequestedBatchSize: sql.NullInt64{Int64: 1000000, Valid: true},
	})

	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	if job.Status != "processing" {
		t.Errorf("Expected status 'processing', got '%s'", job.Status)
	}

	if !job.WorkerID.Valid || job.WorkerID.String != "test-worker-1" {
		t.Errorf("Worker ID not set correctly")
	}

	// Verify expires_at is in the future
	if !job.ExpiresAt.Valid {
		t.Error("ExpiresAt should be set")
	}
}

func TestFindAvailableBatch(t *testing.T) {
	ctx := context.Background()
	db, queries := setupTestDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close failed: %v", err)
		}
	}()

	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28}

	// Create a pending job (insert directly with status=pending)
	_, err := db.ExecContext(ctx, `
		INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status)
		VALUES (?, ?, ?, 'pending')
	`, prefix, 0, 1000000)

	if err != nil {
		t.Fatalf("Failed to insert test job: %v", err)
	}

	// Find available batch
	job, err := queries.FindAvailableBatch(ctx)
	if err != nil {
		t.Fatalf("FindAvailableBatch failed: %v", err)
	}

	if job.Status != "pending" {
		t.Errorf("Expected pending job, got %s", job.Status)
	}
}

func TestUpdateCheckpoint(t *testing.T) {
	ctx := context.Background()
	db, queries := setupTestDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close failed: %v", err)
		}
	}()

	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28}

	// Create a processing job
	job, err := queries.CreateBatch(ctx, CreateBatchParams{
		Prefix28:           prefix,
		NonceStart:         0,
		NonceEnd:           1000000,
		CurrentNonce:       sql.NullInt64{Valid: false},
		WorkerID:           sql.NullString{String: "test-worker-1", Valid: true},
		WorkerType:         sql.NullString{String: "pc", Valid: true},
		Column7:            sql.NullString{String: "3600", Valid: true},
		RequestedBatchSize: sql.NullInt64{Int64: 1000000, Valid: true},
	})

	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// Update checkpoint
	err = queries.UpdateCheckpoint(ctx, UpdateCheckpointParams{
		CurrentNonce: sql.NullInt64{Int64: 500000, Valid: true},
		KeysScanned:  sql.NullInt64{Int64: 500000, Valid: true},
		ID:           job.ID,
		WorkerID:     sql.NullString{String: "test-worker-1", Valid: true},
	})

	if err != nil {
		t.Errorf("UpdateCheckpoint failed: %v", err)
	}

	// Verify update
	updated, err := queries.GetJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJobByID failed: %v", err)
	}

	if !updated.CurrentNonce.Valid || updated.CurrentNonce.Int64 != 500000 {
		t.Errorf("CurrentNonce not updated correctly")
	}

	if !updated.KeysScanned.Valid || updated.KeysScanned.Int64 != 500000 {
		t.Errorf("KeysScanned not updated correctly")
	}
}

func TestCompleteBatch(t *testing.T) {
	ctx := context.Background()
	db, queries := setupTestDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close failed: %v", err)
		}
	}()

	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28}

	// Create a processing job
	job, err := queries.CreateBatch(ctx, CreateBatchParams{
		Prefix28:           prefix,
		NonceStart:         0,
		NonceEnd:           1000000,
		CurrentNonce:       sql.NullInt64{Valid: false},
		WorkerID:           sql.NullString{String: "test-worker-1", Valid: true},
		WorkerType:         sql.NullString{String: "pc", Valid: true},
		Column7:            sql.NullString{String: "3600", Valid: true},
		RequestedBatchSize: sql.NullInt64{Int64: 1000000, Valid: true},
	})

	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// Complete batch
	err = queries.CompleteBatch(ctx, CompleteBatchParams{
		KeysScanned: sql.NullInt64{Int64: 1000000, Valid: true},
		ID:          job.ID,
		WorkerID:    sql.NullString{String: "test-worker-1", Valid: true},
	})

	if err != nil {
		t.Errorf("CompleteBatch failed: %v", err)
	}

	// Verify completion
	completed, err := queries.GetJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJobByID failed: %v", err)
	}

	if completed.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", completed.Status)
	}

	if !completed.CompletedAt.Valid {
		t.Error("CompletedAt should be set")
	}
}

func TestUTCTimestamps(t *testing.T) {
	ctx := context.Background()
	db, queries := setupTestDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close failed: %v", err)
		}
	}()

	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28}

	// Create job
	job, err := queries.CreateBatch(ctx, CreateBatchParams{
		Prefix28:           prefix,
		NonceStart:         0,
		NonceEnd:           1000000,
		CurrentNonce:       sql.NullInt64{Valid: false},
		WorkerID:           sql.NullString{String: "test-worker-1", Valid: true},
		WorkerType:         sql.NullString{String: "pc", Valid: true},
		Column7:            sql.NullString{String: "3600", Valid: true},
		RequestedBatchSize: sql.NullInt64{Int64: 1000000, Valid: true},
	})

	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// Verify created_at is UTC
	if job.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt should be UTC, got %s", job.CreatedAt.Location())
	}

	// Verify expires_at is UTC
	if job.ExpiresAt.Valid && job.ExpiresAt.Time.Location() != time.UTC {
		t.Errorf("ExpiresAt should be UTC, got %s", job.ExpiresAt.Time.Location())
	}
}
