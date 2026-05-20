package link

import "testing"

func TestShortCodeRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   uint64
	}{
		{name: "small id", id: 1},
		{name: "six char body", id: 56800235583},
		{name: "snowflake like id", id: 5301842092032},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := NewShortCode(tt.id)
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

func TestValidateShortCodeRejectsTampering(t *testing.T) {
	t.Parallel()

	code := NewShortCode(42)
	tampered := code[:len(code)-1] + "0"
	if tampered == code {
		tampered = code[:len(code)-1] + "1"
	}

	if err := ValidateShortCode(tampered); err == nil {
		t.Fatalf("ValidateShortCode(%q) accepted a tampered code", tampered)
	}
}

func TestValidateShortCodeRejectsMalformedCode(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"", "x", "bad-code"} {
		t.Run(code, func(t *testing.T) {
			if err := ValidateShortCode(code); err == nil {
				t.Fatalf("ValidateShortCode(%q) accepted malformed code", code)
			}
		})
	}
}
