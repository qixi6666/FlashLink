package filter

import (
	"context"
	"testing"
)

func TestMemoryRebuild(t *testing.T) {
	t.Parallel()

	filter := NewMemory()
	source := fakeCodeSource{codes: []string{"a", "b"}}
	if err := filter.Rebuild(context.Background(), source, 1); err != nil {
		t.Fatalf("Rebuild returned error: %v", err)
	}

	for _, code := range source.codes {
		ok, err := filter.MightContain(context.Background(), code)
		if err != nil {
			t.Fatalf("MightContain returned error: %v", err)
		}
		if !ok {
			t.Fatalf("MightContain(%q) = false, want true", code)
		}
	}
}

type fakeCodeSource struct {
	codes []string
}

func (s fakeCodeSource) ListActiveCodes(_ context.Context, batchSize int, handle func([]string) error) error {
	for i := 0; i < len(s.codes); i += batchSize {
		end := i + batchSize
		if end > len(s.codes) {
			end = len(s.codes)
		}
		if err := handle(s.codes[i:end]); err != nil {
			return err
		}
	}
	return nil
}
