package keywordimg

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadImageSendsContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "绿叶脑袋.jpeg")
	want := []byte{0xff, 0xd8, 0x00, 0x01, 0xff, 0xd9}
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatal(err)
	}
	segment, err := readImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if segment.Type != "image" || !strings.HasPrefix(segment.Data["file"], "base64://") {
		t.Fatalf("expected embedded image, got %v", segment)
	}
	got, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(segment.Data["file"], "base64://"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("image contents changed: %x, error: %v", got, err)
	}
	if _, err := readImage(filepath.Join(t.TempDir(), "missing.jpeg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing-file error, got %v", err)
	}
}

func TestStoreImageReplacement(t *testing.T) {
	oldData, oldDataFile := keywordData, dataFile
	t.Cleanup(func() {
		keywordData, dataFile = oldData, oldDataFile
	})
	keywordData = make(map[string]string)
	dir := t.TempDir()
	dataFile = filepath.Join(dir, "keywords.json")
	keyword := "绿叶脑袋"
	path := filepath.Join(dir, keyword+".jpeg")
	for _, content := range []string{"original image", "replacement image"} {
		if err := storeImage(keyword, path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("saved image missing or incorrect: %q, error: %v", got, err)
		}
	}

	// A failed write must preserve the existing image and keyword mapping.
	if err := storeImage(keyword, filepath.Join(dir, "missing", "image.png"), []byte("bad")); err == nil {
		t.Fatal("expected write failure")
	}
	if keywordData[keyword] != path {
		t.Fatal("failed write changed keyword mapping")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "replacement image" {
		t.Fatalf("failed write damaged existing image: %q, error: %v", got, err)
	}

	// Changing image format removes only the old file and persists the new path.
	newPath := filepath.Join(dir, keyword+".png")
	if err := storeImage(keyword, newPath, []byte("new format")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old image should be removed: %v", err)
	}
	if got, err := os.ReadFile(newPath); err != nil || string(got) != "new format" {
		t.Fatalf("new image missing or incorrect: %q, error: %v", got, err)
	}
	data, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	var saved []KeywordImage
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].Keyword != keyword || saved[0].ImageURL != newPath {
		t.Fatalf("incorrect persisted mapping: %+v", saved)
	}
}
