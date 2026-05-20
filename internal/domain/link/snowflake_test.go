package link

import (
	"testing"
	"time"
)

func TestSnowflakeNextIDAndParse(t *testing.T) {
	t.Parallel()

	sf, err := NewSnowflake(7)
	if err != nil {
		t.Fatalf("NewSnowflake returned error: %v", err)
	}
	fixed := snowflakeEpoch.Add(1500 * time.Millisecond)
	sf.now = func() time.Time { return fixed }

	first := sf.NextID()
	second := sf.NextID()
	if second <= first {
		t.Fatalf("second id %d must be greater than first %d", second, first)
	}

	parts := ParseSnowflake(first)
	if !parts.Timestamp.Equal(fixed.UTC()) {
		t.Fatalf("timestamp = %s, want %s", parts.Timestamp, fixed.UTC())
	}
	if parts.NodeID != 7 {
		t.Fatalf("node id = %d, want 7", parts.NodeID)
	}
	if parts.Sequence != 0 {
		t.Fatalf("sequence = %d, want 0", parts.Sequence)
	}
}

func TestNewSnowflakeRejectsInvalidNode(t *testing.T) {
	t.Parallel()

	if _, err := NewSnowflake(snowflakeMaxNode + 1); err == nil {
		t.Fatal("NewSnowflake accepted invalid node id")
	}
}
