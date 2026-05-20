package link

import "testing"

func TestBase62RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   uint64
		want string
	}{
		{name: "zero", id: 0, want: "0"},
		{name: "single digit upper bound", id: 61, want: "z"},
		{name: "two digits", id: 62, want: "10"},
		{name: "large value", id: 56800235583, want: "zzzzzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeBase62(tt.id)
			if got != tt.want {
				t.Fatalf("EncodeBase62(%d) = %q, want %q", tt.id, got, tt.want)
			}

			decoded, err := DecodeBase62(got)
			if err != nil {
				t.Fatalf("DecodeBase62(%q) returned error: %v", got, err)
			}
			if decoded != tt.id {
				t.Fatalf("DecodeBase62(%q) = %d, want %d", got, decoded, tt.id)
			}
		})
	}
}

func TestDecodeBase62RejectsInvalidValue(t *testing.T) {
	t.Parallel()

	if _, err := DecodeBase62("abc-123"); err == nil {
		t.Fatal("DecodeBase62 accepted invalid input")
	}
}
