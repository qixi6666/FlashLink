package link

import "testing"

func TestShortCodeRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   uint64
	}{
		{name: "zero id", id: 0},
		{name: "small id", id: 1},
		{name: "six char body", id: 56800235583},
		{name: "snowflake like id", id: 5301842092032},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := NewShortCode(tt.id)
			if code != EncodeBase62(tt.id) {
				t.Fatalf("NewShortCode(%d) = %q, want %q", tt.id, code, EncodeBase62(tt.id))
			}
			if err := ValidateShortCode(code); err != nil {
				t.Fatalf("ValidateShortCode(%q) returned error: %v", code, err)
			}

			got, err := IDFromShortCode(code)
			if err != nil {
				t.Fatalf("IDFromShortCode(%q) returned error: %v", code, err)
			}
			if got != tt.id {
				t.Fatalf("IDFromShortCode(%q) = %d, want %d", code, got, tt.id)
			}
		})
	}
}

func TestValidateShortCodeAcceptsBase62Code(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"0", "x", "W7E"} {
		t.Run(code, func(t *testing.T) {
			if err := ValidateShortCode(code); err != nil {
				t.Fatalf("ValidateShortCode(%q) returned error: %v", code, err)
			}
		})
	}
}

func TestValidateShortCodeRejectsMalformedCode(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"", "bad-code"} {
		t.Run(code, func(t *testing.T) {
			if err := ValidateShortCode(code); err == nil {
				t.Fatalf("ValidateShortCode(%q) accepted malformed code", code)
			}
		})
	}
}
