package fortune

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCachedImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fortune.png")
	want := []byte("image contents")
	calls := 0
	generate := func(w io.Writer) error {
		calls++
		_, err := w.Write(want)
		return err
	}
	for i := 0; i < 2; i++ {
		got, err := cachedImage(path, generate)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("image contents = %q, error = %v", got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("generated %d times, expected once", calls)
	}
}

func TestCachedImageGenerationFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fortune.png")
	failure := errors.New("render failed")
	_, err := cachedImage(path, func(w io.Writer) error {
		_, _ = w.Write([]byte("incomplete image"))
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("expected render error, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed rendering left a cached image: %v", err)
	}
}
