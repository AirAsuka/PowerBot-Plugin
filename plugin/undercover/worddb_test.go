package undercover

import (
	"strings"
	"testing"
)

func TestWordLibrarySeedsAndReportsStats(t *testing.T) {
	library := newWordLibrary(t.TempDir() + "/words.db")
	if err := library.initialize(); err != nil {
		t.Fatal(err)
	}
	rows, total, err := library.list(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 10 || total != len(builtinPairs()) {
		t.Fatalf("got page=%d total=%d, want page=10 total=%d", len(rows), total, len(builtinPairs()))
	}
	stats, statsTotal, enabled, err := library.stats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != len(builtinWordGroups) || statsTotal != total || enabled != total {
		t.Fatalf("unexpected stats: categories=%d total=%d enabled=%d", len(stats), statsTotal, enabled)
	}
}

func TestWordLibraryManagementAndDraw(t *testing.T) {
	library := newWordLibrary(t.TempDir() + "/words.db")
	id, err := library.add("苹果手机", "安卓手机", "数码", 3, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("invalid inserted id: %d", id)
	}
	if _, err := library.add("安卓手机", "苹果手机", "数码", 3, 12345); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("reverse duplicate returned %v", err)
	}
	if _, err := library.db.Exec(`UPDATE undercover_words SET enabled = 0 WHERE id <> ?`, id); err != nil {
		t.Fatal(err)
	}
	pair, err := library.randomPair()
	if err != nil {
		t.Fatal(err)
	}
	if pair.Civilian != "苹果手机" || pair.Undercover != "安卓手机" {
		t.Fatalf("draw returned %+v", pair)
	}
	var useCount struct{ Count int64 }
	if err := library.db.Query(`SELECT use_count FROM undercover_words WHERE id = ?`, &useCount, id); err != nil {
		t.Fatal(err)
	}
	if useCount.Count != 1 {
		t.Fatalf("use_count=%d, want 1", useCount.Count)
	}
	if err := library.setEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := library.randomPair(); err == nil {
		t.Fatal("draw succeeded with no enabled words")
	}
}

func TestValidateWordAndCanonicalPairKey(t *testing.T) {
	if canonicalPairKey(" QQ ", "微信") != canonicalPairKey("微信", "qq") {
		t.Fatal("canonical pair key depends on order or case")
	}
	for _, word := range []string{"", "含 空格", "含|竖线", "一二三四五六七八九十一二三"} {
		if err := validateWord(word); err == nil {
			t.Fatalf("validateWord(%q) succeeded", word)
		}
	}
}
