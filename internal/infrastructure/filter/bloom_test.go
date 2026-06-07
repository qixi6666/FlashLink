package filter

import "testing"

func TestNewRedisBloomAppliesDefaults(t *testing.T) {
	t.Parallel()

	filter := NewRedisBloom(nil, RedisBloomOptions{})
	if filter.key != defaultBloomFilterKey {
		t.Fatalf("key = %q, want %q", filter.key, defaultBloomFilterKey)
	}
	if filter.capacity != int64(defaultBloomFilterCapacity) {
		t.Fatalf("capacity = %d, want %d", filter.capacity, defaultBloomFilterCapacity)
	}
	if filter.errorRate != defaultBloomFilterErrorRate {
		t.Fatalf("errorRate = %f, want %f", filter.errorRate, defaultBloomFilterErrorRate)
	}
}

func TestNewRedisBloomRejectsInvalidSizingOptions(t *testing.T) {
	t.Parallel()

	filter := NewRedisBloom(nil, RedisBloomOptions{
		Capacity:  0,
		ErrorRate: 2,
	})
	if filter.capacity != int64(defaultBloomFilterCapacity) {
		t.Fatalf("capacity = %d, want %d", filter.capacity, defaultBloomFilterCapacity)
	}
	if filter.errorRate != defaultBloomFilterErrorRate {
		t.Fatalf("errorRate = %f, want %f", filter.errorRate, defaultBloomFilterErrorRate)
	}
}

func TestDedupeStrings(t *testing.T) {
	t.Parallel()

	got := dedupeStrings([]string{"main", "temp", "main", "", "temp"})
	want := []string{"main", "temp"}
	if len(got) != len(want) {
		t.Fatalf("dedupeStrings length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupeStrings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStringsToInterfaces(t *testing.T) {
	t.Parallel()

	got := stringsToInterfaces([]string{"a", "b"})
	if len(got) != 2 {
		t.Fatalf("stringsToInterfaces length = %d, want 2", len(got))
	}
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("stringsToInterfaces = %#v, want a,b", got)
	}
}
