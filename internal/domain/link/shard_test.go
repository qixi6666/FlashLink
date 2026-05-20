package link

import "testing"

func TestShardRoutingFromIDAndCode(t *testing.T) {
	t.Parallel()

	for id := uint64(0); id < 128; id++ {
		code := NewShortCode(id)

		fromCode, err := ShardIndexFromCode(code)
		if err != nil {
			t.Fatalf("ShardIndexFromCode(%q) returned error: %v", code, err)
		}

		fromID := ShardIndexFromID(id)
		if fromCode != fromID {
			t.Fatalf("shard mismatch for id %d code %q: from code %d, from id %d", id, code, fromCode, fromID)
		}
	}
}

func TestShardTableName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		index   int
		want    string
		wantErr bool
	}{
		{name: "first", index: 0, want: "short_link_00"},
		{name: "last", index: 15, want: "short_link_15"},
		{name: "negative", index: -1, wantErr: true},
		{name: "too large", index: 16, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShardTableName(tt.index)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ShardTableName(%d) returned nil error", tt.index)
				}
				return
			}
			if err != nil {
				t.Fatalf("ShardTableName(%d) returned error: %v", tt.index, err)
			}
			if got != tt.want {
				t.Fatalf("ShardTableName(%d) = %q, want %q", tt.index, got, tt.want)
			}
		})
	}
}
