package undercover

import (
	"strings"
	"sync"
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
	var state struct{ Enabled, Count int64 }
	if err := library.db.Query(`SELECT enabled, use_count FROM undercover_words WHERE id = ?`, &state, id); err != nil {
		t.Fatal(err)
	}
	if state.Enabled != 0 || state.Count != 1 {
		t.Fatalf("draw did not soft-delete word: %+v", state)
	}
	if err := library.setEnabled(id, true); err == nil || !strings.Contains(err.Error(), "已使用") {
		t.Fatalf("re-enabling used word returned %v", err)
	}
	if _, err := library.randomPair(); err == nil {
		t.Fatal("draw reused an exhausted pool")
	}
	if _, err := library.add("安卓手机", "苹果手机", "数码", 3, 12345); err == nil {
		t.Fatal("re-added a soft-deleted word pair")
	}
	_, total, enabled, err := library.stats()
	if err != nil || total != len(builtinPairs())+1 || enabled != 0 {
		t.Fatalf("unexpected exhausted stats: total=%d enabled=%d err=%v", total, enabled, err)
	}
}

func TestWordLibraryUpgradePreservesUsedAndDisabledWords(t *testing.T) {
	path := t.TempDir() + "/words.db"
	library := newWordLibrary(path)
	if err := library.initialize(); err != nil {
		t.Fatal(err)
	}
	// 模拟旧版的已用启用词、手动禁用词和升级后新增的内置词。
	if _, err := library.db.Exec(`UPDATE undercover_words SET use_count = 3 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := library.setEnabled(2, false); err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.Exec(`DELETE FROM undercover_words WHERE id = 3`); err != nil {
		t.Fatal(err)
	}
	if err := library.db.Close(); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 2; restart++ {
		library = newWordLibrary(path)
		rows, total, err := library.list(1, len(builtinPairs()))
		if err != nil {
			t.Fatal(err)
		}
		if total != len(builtinPairs()) || rows[0].ID != 1 || rows[0].Enabled != 0 || rows[0].UseCount != 3 || rows[1].Enabled != 0 {
			t.Fatalf("upgrade lost word state: total=%d first=%+v second=%+v", total, rows[0], rows[1])
		}
		_, _, enabled, err := library.stats()
		if err != nil || enabled != total-2 {
			t.Fatalf("enabled=%d, want %d; err=%v", enabled, total-2, err)
		}
		if err := library.db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWordLibraryUnusedWordsCanBeReenabled(t *testing.T) {
	library := newWordLibrary(t.TempDir() + "/words.db")
	if err := library.setEnabled(1, false); err != nil {
		t.Fatal(err)
	}
	if err := library.setEnabled(1, true); err != nil {
		t.Fatal(err)
	}
	if err := library.setEnabled(999999, true); err == nil {
		t.Fatal("enabled a missing word")
	}
	_, total, enabled, err := library.stats()
	if err != nil || enabled != total {
		t.Fatalf("enabled=%d total=%d err=%v", enabled, total, err)
	}
}

func TestWordLibraryConcurrentDrawsDoNotRepeat(t *testing.T) {
	library := newWordLibrary(t.TempDir() + "/words.db")
	if err := library.initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := library.db.Exec(`UPDATE undercover_words SET enabled = 0 WHERE id > 16`); err != nil {
		t.Fatal(err)
	}
	results := make(chan wordPair, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pair, err := library.randomPair()
			if err != nil {
				t.Error(err)
				return
			}
			results <- pair
		}()
	}
	wg.Wait()
	close(results)
	seen := make(map[string]bool)
	for pair := range results {
		key := canonicalPairKey(pair.Civilian, pair.Undercover)
		if seen[key] {
			t.Fatalf("repeated pair: %+v", pair)
		}
		seen[key] = true
	}
	if len(seen) != 16 {
		t.Fatalf("drew %d distinct pairs, want 16", len(seen))
	}
	if err := library.db.Close(); err != nil {
		t.Fatal(err)
	}
	library = newWordLibrary(library.path)
	if _, err := library.randomPair(); err == nil {
		t.Fatal("restart restored exhausted words")
	}
	// 词池耗尽后添加新词，应只抽中新词。
	id, err := library.add("测试甲", "测试乙", "测试", 1, 123)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := library.randomPair()
	if err != nil || pair.Civilian != "测试甲" || pair.Undercover != "测试乙" {
		t.Fatalf("new word #%d: pair=%+v err=%v", id, pair, err)
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
