package emojimix

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// ---- metadata.json 结构体 (对应 emoji-kitchen-main/src/Components/types.tsx) ----

type emojiMetadata struct {
	KnownSupportedEmoji []string             `json:"knownSupportedEmoji"`
	Data               map[string]*emojiData `json:"data"`
}

type emojiData struct {
	Alt            string                        `json:"alt"`
	EmojiCodepoint string                        `json:"emojiCodepoint"`
	GBoardOrder    int                           `json:"gBoardOrder"`
	Combinations   map[string][]emojiCombination `json:"combinations"`
}

type emojiCombination struct {
	GStaticUrl          string `json:"gStaticUrl"`
	Alt                 string `json:"alt"`
	LeftEmojiCodepoint  string `json:"leftEmojiCodepoint"`
	RightEmojiCodepoint string `json:"rightEmojiCodepoint"`
	Date                string `json:"date"`
	IsLatest            bool   `json:"isLatest"`
	GBoardOrder         int    `json:"gBoardOrder"`
}

var (
	// 合成索引: key = "normalizedCP1_normalizedCP2" (字典序) -> gStaticUrl
	mixIndex map[string]string
	// rune 序列 -> codepoint 字符串，用于从用户输入识别 emoji
	runeToCP map[string]string
	once     sync.Once

	// SQLite 数据库
	dbPath  string
	db      *sql.DB
	dbMutex sync.Mutex
)

func init() {
	loadData()

	control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "合成emoji",
		Help:             "- [emoji][emoji]",
	}).OnMessage(match).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			cps := ctx.State["emojimix"].([]string)
			url := lookupCombination(cps[0], cps[1])
			if url != "" {
				ctx.SendChain(message.Image(url))
				return
			}
			logrus.Debugf("[emojimix] 未找到合成: %s + %s", cps[0], cps[1])
		})
}

// ---- 数据加载 ----

func loadData() {
	once.Do(func() {
		mixIndex = make(map[string]string)
		runeToCP = make(map[string]string)

		dataDir := filepath.Join("data", "emojimix")
		jsonPath := filepath.Join(dataDir, "metadata.json")
		dbPath = filepath.Join(dataDir, "emojimix.db")

		// 确保目录存在
		_ = os.MkdirAll(dataDir, 0755)

		// 检查 SQLite 数据库是否存在
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			// 数据库不存在，从 JSON 加载并创建数据库
			logrus.Infoln("[emojimix] 首次加载，正在从 JSON 构建数据库...")
			if err := buildDBFromJSON(jsonPath); err != nil {
				logrus.Errorf("[emojimix] 从JSON构建数据库失败: %v", err)
				// 回退到纯内存模式
				loadFromJSONOnly(jsonPath)
				return
			}
			logrus.Infoln("[emojimix] 数据库构建完成")
		}

		// 从 SQLite 数据库加载到内存
		if err := loadFromDB(); err != nil {
			logrus.Errorf("[emojimix] 从数据库加载失败: %v", err)
			// 回退到直接读取 JSON
			loadFromJSONOnly(jsonPath)
			return
		}

		logrus.Infof("[emojimix] 加载完成: %d 个 emoji, %d 条合成索引",
			len(runeToCP), len(mixIndex))
	})
}

// loadFromJSONOnly 回退方案：直接从 JSON 加载到内存（不创建数据库）
func loadFromJSONOnly(jsonPath string) {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		logrus.Errorf("[emojimix] 读取 metadata 失败: %v", err)
		return
	}

	var meta emojiMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		logrus.Errorf("[emojimix] 解析 metadata 失败: %v", err)
		return
	}

	buildMapsFromMeta(&meta)
}

// buildDBFromJSON 从 JSON 文件构建 SQLite 数据库
func buildDBFromJSON(jsonPath string) error {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	var meta emojiMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("解析JSON失败: %w", err)
	}

	// 创建数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	// 创建表
	schema := `
	CREATE TABLE IF NOT EXISTS emoji_runes (
		runes TEXT PRIMARY KEY,
		codepoint TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS emoji_mix (
		key TEXT PRIMARY KEY,
		url TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_mix_url ON emoji_mix(key);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("创建表失败: %w", err)
	}

	// 开启事务批量插入
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}

	// 插入 rune 映射
	stmtRune, err := tx.Prepare("INSERT OR REPLACE INTO emoji_runes (runes, codepoint) VALUES (?, ?)")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("准备rune语句失败: %w", err)
	}
	defer stmtRune.Close()

	// 插入混合索引
	stmtMix, err := tx.Prepare("INSERT OR REPLACE INTO emoji_mix (key, url) VALUES (?, ?)")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("准备mix语句失败: %w", err)
	}
	defer stmtMix.Close()

	// 构建并插入数据
	for _, cp := range meta.KnownSupportedEmoji {
		runes := cpToRunes(cp)
		runeStr := string(runes)
		stmtRune.Exec(runeStr, cp)

		// 同时存储去掉 FE0F/FE0E 的版本
		stripped := stripVS(runes)
		if len(stripped) != len(runes) {
			stmtRune.Exec(string(stripped), cp)
		}
	}

	for cp1, data := range meta.Data {
		n1 := normalizeCP(cp1)
		for cp2, combos := range data.Combinations {
			n2 := normalizeCP(cp2)
			for _, c := range combos {
				if c.IsLatest && c.GStaticUrl != "" {
					key := sortedPair(n1, n2)
					stmtMix.Exec(key, c.GStaticUrl)
					break
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	// 构建内存索引
	buildMapsFromMeta(&meta)

	return nil
}

// loadFromDB 从 SQLite 数据库加载到内存
func loadFromDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}

	// 设置连接池
	db.SetMaxOpenConns(1)

	// 加载 rune 映射
	rows, err := db.Query("SELECT runes, codepoint FROM emoji_runes")
	if err != nil {
		return fmt.Errorf("查询runes失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var runes, codepoint string
		if err := rows.Scan(&runes, &codepoint); err != nil {
			continue
		}
		runeToCP[runes] = codepoint
	}

	// 加载混合索引
	rows, err = db.Query("SELECT key, url FROM emoji_mix")
	if err != nil {
		return fmt.Errorf("查询mix失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key, url string
		if err := rows.Scan(&key, &url); err != nil {
			continue
		}
		mixIndex[key] = url
	}

	return nil
}

// buildMapsFromMeta 从解析好的 meta 数据构建内存索引
func buildMapsFromMeta(meta *emojiMetadata) {
	// 1) 构建 rune 序列 -> codepoint 映射
	for _, cp := range meta.KnownSupportedEmoji {
		runes := cpToRunes(cp)
		runeToCP[string(runes)] = cp
		// 同时存储去掉 FE0F/FE0E 的版本
		stripped := stripVS(runes)
		if len(stripped) != len(runes) {
			runeToCP[string(stripped)] = cp
		}
	}

	// 2) 构建合成索引
	for cp1, data := range meta.Data {
		n1 := normalizeCP(cp1)
		for cp2, combos := range data.Combinations {
			n2 := normalizeCP(cp2)
			for _, c := range combos {
				if c.IsLatest && c.GStaticUrl != "" {
					key := sortedPair(n1, n2)
					if _, ok := mixIndex[key]; !ok {
						mixIndex[key] = c.GStaticUrl
					}
					break
				}
			}
		}
	}
}

// ---- 查询 ----

// lookupCombination 根据两个 emoji 的 codepoint 字符串查找合成图片 URL
func lookupCombination(cp1, cp2 string) string {
	return mixIndex[sortedPair(normalizeCP(cp1), normalizeCP(cp2))]
}

// ---- 消息匹配 ----

func match(ctx *zero.Ctx) bool {
	// 从所有 segment 中提取 emoji codepoint
	cps := make([]string, 0, 2)
	for _, seg := range ctx.Event.Message {
		if cp := segmentToCP(seg); cp != "" {
			cps = append(cps, cp)
		}
	}

	var cp1, cp2 string
	if len(cps) == 2 {
		cp1, cp2 = cps[0], cps[1]
	} else {
		// 兜底: 从 RawMessage 中分割两个 emoji
		cp1, cp2 = splitTwoEmoji([]rune(ctx.Event.RawMessage))
	}

	if cp1 != "" && cp2 != "" {
		ctx.State["emojimix"] = []string{cp1, cp2}
		return true
	}
	return false
}

// segmentToCP 从消息 segment 中提取 emoji codepoint 字符串
func segmentToCP(seg message.Segment) string {
	switch seg.Type {
	case "text":
		runes := []rune(seg.Data["text"])
		// 去掉空白和零值
		filtered := make([]rune, 0, len(runes))
		for _, r := range runes {
			if r > 0 && r != ' ' && r != '\t' && r != '\n' && r != '\r' {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			return ""
		}
		// 先尝试完整 rune 序列匹配已知 emoji
		if cp, ok := runeToCP[string(filtered)]; ok {
			return cp
		}
		// 再尝试去掉 Variation Selector 后匹配
		if cp, ok := runeToCP[string(stripVS(filtered))]; ok {
			return cp
		}
		// 兜底: 如果只有单个 rune 且看起来像 emoji，直接用 hex
		if len(filtered) == 1 && isEmoji(filtered[0]) {
			return fmt.Sprintf("%x", filtered[0])
		}
	case "face":
		id, _ := strconv.Atoi(seg.Data["id"])
		if r, ok := message.Emoji[id]; ok {
			return fmt.Sprintf("%x", r)
		}
	}
	return ""
}

// splitTwoEmoji 尝试将 rune 序列分割成恰好两个已知 emoji
func splitTwoEmoji(rawRunes []rune) (string, string) {
	// 过滤零值和空白
	runes := make([]rune, 0, len(rawRunes))
	for _, r := range rawRunes {
		if r > 0 && r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			runes = append(runes, r)
		}
	}
	if len(runes) < 2 {
		return "", ""
	}

	// 尝试原始序列分割
	if cp1, cp2 := trySplit(runes); cp1 != "" {
		return cp1, cp2
	}

	// 尝试去掉 Variation Selector 后分割
	stripped := stripVS(runes)
	if len(stripped) != len(runes) && len(stripped) >= 2 {
		return trySplit(stripped)
	}

	return "", ""
}

// trySplit 遍历所有可能的分割点，查找两个已知 emoji
func trySplit(runes []rune) (string, string) {
	n := len(runes)
	for i := 1; i < n; i++ {
		leftCP, leftOK := runeToCP[string(runes[:i])]
		rightCP, rightOK := runeToCP[string(runes[i:])]
		if leftOK && rightOK {
			return leftCP, rightCP
		}
	}
	return "", ""
}

// ---- 工具函数 ----

// cpToRunes 将 codepoint 字符串 (如 "1f636-200d-1f32b-fe0f") 转为 rune 切片
func cpToRunes(cp string) []rune {
	parts := strings.Split(cp, "-")
	runes := make([]rune, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseInt(p, 16, 32)
		if err == nil {
			runes = append(runes, rune(v))
		}
	}
	return runes
}

// stripVS 去掉 rune 切片中的 Variation Selector (U+FE0F / U+FE0E)
func stripVS(runes []rune) []rune {
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		if r != 0xFE0F && r != 0xFE0E {
			out = append(out, r)
		}
	}
	return out
}

// normalizeCP 去掉 codepoint 字符串中的 fe0f / fe0e 变体选择符
func normalizeCP(cp string) string {
	parts := strings.Split(cp, "-")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "fe0f" && p != "fe0e" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "-")
}

// sortedPair 生成字典序排列的配对 key: "smaller_larger"
func sortedPair(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "_" + b
}

// isEmoji 简单判断 rune 是否可能是 emoji
func isEmoji(r rune) bool {
	return r > 0x2000
}
