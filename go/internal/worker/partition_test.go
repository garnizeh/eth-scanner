package worker

import (
	"testing"
)

func TestPartitionNonceRange_100_4(t *testing.T) {
	parts := PartitionNonceRange(0, 99, 4)
	if len(parts) != 4 {
		t.Fatalf("expected 4 partitions, got %d", len(parts))
	}
	want := []NonceRange{{0, 24}, {25, 49}, {50, 74}, {75, 99}}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("partition %d mismatch: got %+v want %+v", i, parts[i], want[i])
		}
	}
}

func TestPartitionNonceRange_101_4(t *testing.T) {
	parts := PartitionNonceRange(0, 100, 4)
	if len(parts) != 4 {
		t.Fatalf("expected 4 partitions, got %d", len(parts))
	}
	want := []NonceRange{{0, 25}, {26, 50}, {51, 75}, {76, 100}}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("partition %d mismatch: got %+v want %+v", i, parts[i], want[i])
		}
	}
}

func TestPartitionNonceRange_SmallerThanWorkers(t *testing.T) {
	parts := PartitionNonceRange(0, 2, 4)
	if len(parts) != 3 {
		t.Fatalf("expected 3 partitions, got %d", len(parts))
	}
	want := []NonceRange{{0, 0}, {1, 1}, {2, 2}}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("partition %d mismatch: got %+v want %+v", i, parts[i], want[i])
		}
	}
}

func TestPartitionNonceRange_SingleNonceManyWorkers(t *testing.T) {
	parts := PartitionNonceRange(42, 42, 10)
	if len(parts) != 1 {
		t.Fatalf("expected 1 partition, got %d", len(parts))
	}
	if parts[0] != (NonceRange{42, 42}) {
		t.Fatalf("unexpected partition: %+v", parts[0])
	}
}

func TestPartitionNonceRange_ContiguousAndSum(t *testing.T) {
	start, end := uint32(5), uint32(205)
	parts := PartitionNonceRange(start, end, 7)
	if len(parts) == 0 {
		t.Fatalf("expected partitions, got none")
	}
	// Check contiguity and full coverage
	if parts[0].Start != start {
		t.Fatalf("first partition start mismatch: got %d want %d", parts[0].Start, start)
	}
	if parts[len(parts)-1].End != end {
		t.Fatalf("last partition end mismatch: got %d want %d", parts[len(parts)-1].End, end)
	}
	// Check no gaps and no overlaps
	var total uint64
	for i := range parts {
		r := parts[i]
		if r.Start > r.End {
			t.Fatalf("invalid partition with start>End: %+v", r)
		}
		total += uint64(r.End) - uint64(r.Start) + 1
		if i > 0 {
			if parts[i-1].End+1 != r.Start {
				t.Fatalf("gap or overlap between %v and %v", parts[i-1], r)
			}
		}
	}
	expected := uint64(end) - uint64(start) + 1
	if total != expected {
		t.Fatalf("total nonces mismatch: got %d want %d", total, expected)
	}
}

func TestPartitionNonceRange_StartGreaterThanEnd(t *testing.T) {
	parts := PartitionNonceRange(10, 5, 4)
	if parts != nil {
		t.Fatalf("expected nil partitions when start > end, got %+v", parts)
	}
}

func TestPartitionNonceRange_NumWorkersNonPositive(t *testing.T) {
	partsZero := PartitionNonceRange(0, 9, 0)
	partsNeg := PartitionNonceRange(0, 9, -3)
	if len(partsZero) != 1 {
		t.Fatalf("expected 1 partition for numWorkers=0, got %d", len(partsZero))
	}
	if len(partsNeg) != 1 {
		t.Fatalf("expected 1 partition for numWorkers<0, got %d", len(partsNeg))
	}
	if partsZero[0] != partsNeg[0] {
		t.Fatalf("expected same partition for 0 and negative workers: %v vs %v", partsZero[0], partsNeg[0])
	}
}
