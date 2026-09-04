package undercover

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	sqlite "github.com/FloatTech/sqlite"
)

// wordRecord 对应 SQLite 的 undercover_words 表。
type wordRecord struct {
	ID         int64  `db:"id"`
	WordA      string `db:"word_a"`
	WordB      string `db:"word_b"`
	PairKey    string `db:"pair_key"`
	Category   string `db:"category"`
	Difficulty int    `db:"difficulty"`
	Enabled    int    `db:"enabled"`
	UseCount   int64  `db:"use_count"`
	CreatedBy  int64  `db:"created_by"`
	CreatedAt  int64  `db:"created_at"`
}

type categoryStat struct {
	Category string
	Total    int
	Enabled  int
}

type wordLibrary struct {
	mu          sync.Mutex
	path        string
	db          sqlite.Sqlite
	initialized bool
}

func newWordLibrary(path string) *wordLibrary {
	return &wordLibrary{path: path}
}

func (l *wordLibrary) initialize() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.initializeLocked()
}

func (l *wordLibrary) initializeLocked() error {
	if l.initialized {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("创建词库目录失败: %w", err)
	}
	l.db = sqlite.New(l.path)
	if err := l.db.Open(time.Hour); err != nil {
		return fmt.Errorf("打开词库失败: %w", err)
	}
	schema := `CREATE TABLE IF NOT EXISTS undercover_words (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		word_a TEXT NOT NULL,
		word_b TEXT NOT NULL,
		pair_key TEXT NOT NULL UNIQUE,
		category TEXT NOT NULL DEFAULT '通用',
		difficulty INTEGER NOT NULL DEFAULT 2 CHECK (difficulty BETWEEN 1 AND 3),
		enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
		use_count INTEGER NOT NULL DEFAULT 0,
		created_by INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	)`
	if _, err := l.db.Exec(schema); err != nil {
		return fmt.Errorf("创建词库表失败: %w", err)
	}
	if _, err := l.db.Exec(`CREATE INDEX IF NOT EXISTS idx_undercover_words_pool ON undercover_words(enabled, difficulty, category)`); err != nil {
		return fmt.Errorf("创建词库索引失败: %w", err)
	}
	if _, err := l.db.Exec(`CREATE INDEX IF NOT EXISTS idx_undercover_words_usage ON undercover_words(use_count)`); err != nil {
		return fmt.Errorf("创建词库索引失败: %w", err)
	}
	now := time.Now().Unix()
	for _, group := range builtinWordGroups {
		for _, pair := range group.Pairs {
			_, err := l.db.Exec(`INSERT OR IGNORE INTO undercover_words
				(word_a, word_b, pair_key, category, difficulty, enabled, use_count, created_by, created_at)
				VALUES (?, ?, ?, ?, ?, 1, 0, 0, ?)`, pair.Civilian, pair.Undercover,
				canonicalPairKey(pair.Civilian, pair.Undercover), group.Category, group.Difficulty, now)
			if err != nil {
				return fmt.Errorf("写入内置词库失败: %w", err)
			}
		}
	}
	l.initialized = true
	return nil
}

func (l *wordLibrary) randomPair() (wordPair, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.initializeLocked(); err != nil {
		return wordPair{}, err
	}
	var row wordRecord
	err := l.db.Query(`SELECT id, word_a, word_b, pair_key, category, difficulty,
		enabled, use_count, created_by, created_at
		FROM undercover_words WHERE enabled = 1 ORDER BY RANDOM() LIMIT 1`, &row)
	if errors.Is(err, sqlite.ErrNullResult) {
		return wordPair{}, errors.New("词库中没有可用词条，请管理员先添加或启用词条")
	}
	if err != nil {
		return wordPair{}, fmt.Errorf("抽取词条失败: %w", err)
	}
	if _, err := l.db.Exec(`UPDATE undercover_words SET use_count = use_count + 1 WHERE id = ?`, row.ID); err != nil {
		return wordPair{}, fmt.Errorf("更新词条使用次数失败: %w", err)
	}
	return wordPair{Civilian: row.WordA, Undercover: row.WordB}, nil
}

func (l *wordLibrary) add(wordA, wordB, category string, difficulty int, creator int64) (int64, error) {
	wordA = strings.TrimSpace(wordA)
	wordB = strings.TrimSpace(wordB)
	category = strings.TrimSpace(category)
	if category == "" {
		category = "自定义"
	}
	if err := validateWord(wordA); err != nil {
		return 0, fmt.Errorf("词语A%s", err)
	}
	if err := validateWord(wordB); err != nil {
		return 0, fmt.Errorf("词语B%s", err)
	}
	if strings.EqualFold(wordA, wordB) {
		return 0, errors.New("两个词语不能相同")
	}
	if utf8.RuneCountInString(category) > 12 {
		return 0, errors.New("分类不能超过12个字")
	}
	if difficulty < 1 || difficulty > 3 {
		return 0, errors.New("难度只能是1、2或3")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.initializeLocked(); err != nil {
		return 0, err
	}
	key := canonicalPairKey(wordA, wordB)
	var existing struct{ ID int64 }
	queryErr := l.db.Query(`SELECT id FROM undercover_words WHERE pair_key = ?`, &existing, key)
	if queryErr == nil {
		return 0, fmt.Errorf("该词对已存在，ID为%d", existing.ID)
	}
	if !errors.Is(queryErr, sqlite.ErrNullResult) {
		return 0, fmt.Errorf("检查重复词条失败: %w", queryErr)
	}
	result, err := l.db.Exec(`INSERT INTO undercover_words
		(word_a, word_b, pair_key, category, difficulty, enabled, use_count, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, 1, 0, ?, ?)`, wordA, wordB, key, category, difficulty, creator, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("添加词条失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("读取新词条ID失败: %w", err)
	}
	return id, nil
}

func (l *wordLibrary) setEnabled(id int64, enabled bool) error {
	if id <= 0 {
		return errors.New("词条ID必须是正整数")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.initializeLocked(); err != nil {
		return err
	}
	value := 0
	if enabled {
		value = 1
	}
	result, err := l.db.Exec(`UPDATE undercover_words SET enabled = ? WHERE id = ?`, value, id)
	if err != nil {
		return fmt.Errorf("更新词条失败: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取更新结果失败: %w", err)
	}
	if changed == 0 {
		return fmt.Errorf("没有找到ID为%d的词条", id)
	}
	return nil
}

func (l *wordLibrary) list(page, pageSize int) ([]wordRecord, int, error) {
	if page < 1 {
		page = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.initializeLocked(); err != nil {
		return nil, 0, err
	}
	var count struct{ Total int }
	if err := l.db.Query(`SELECT COUNT(*) FROM undercover_words`, &count); err != nil {
		return nil, 0, fmt.Errorf("统计词库失败: %w", err)
	}
	rows := make([]wordRecord, 0, pageSize)
	var row wordRecord
	err := l.db.QueryFor(`SELECT id, word_a, word_b, pair_key, category, difficulty,
		enabled, use_count, created_by, created_at FROM undercover_words
		ORDER BY id LIMIT ? OFFSET ?`, &row, func() error {
		rows = append(rows, row)
		return nil
	}, pageSize, (page-1)*pageSize)
	if err != nil && !errors.Is(err, sqlite.ErrNullResult) {
		return nil, 0, fmt.Errorf("读取词库失败: %w", err)
	}
	return rows, count.Total, nil
}

func (l *wordLibrary) stats() ([]categoryStat, int, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.initializeLocked(); err != nil {
		return nil, 0, 0, err
	}
	var totals struct {
		Total   int
		Enabled int
	}
	if err := l.db.Query(`SELECT COUNT(*), COALESCE(SUM(enabled), 0) FROM undercover_words`, &totals); err != nil {
		return nil, 0, 0, fmt.Errorf("统计词库失败: %w", err)
	}
	stats := make([]categoryStat, 0)
	var stat categoryStat
	err := l.db.QueryFor(`SELECT category, COUNT(*), COALESCE(SUM(enabled), 0)
		FROM undercover_words GROUP BY category ORDER BY category`, &stat, func() error {
		stats = append(stats, stat)
		return nil
	})
	if err != nil && !errors.Is(err, sqlite.ErrNullResult) {
		return nil, 0, 0, fmt.Errorf("读取分类统计失败: %w", err)
	}
	return stats, totals.Total, totals.Enabled, nil
}

func canonicalPairKey(a, b string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a > b {
		a, b = b, a
	}
	return a + "\x1f" + b
}

func validateWord(word string) error {
	length := utf8.RuneCountInString(word)
	if length < 1 || length > 12 {
		return errors.New("长度必须为1—12个字")
	}
	for _, r := range word {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '|' || r == '｜' {
			return errors.New("不能包含空白、控制字符或竖线")
		}
	}
	return nil
}
