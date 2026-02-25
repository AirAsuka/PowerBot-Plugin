package emojimix

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings" 
    "sync"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// Metadata 结构体定义
type EmojiData struct {
	Data map[string]EmojiInfo `json:"data"`
}

type EmojiInfo struct {
	Combinations map[string][]Combination `json:"combinations"`
}

type Combination struct {
	GStaticUrl string `json:"gStaticUrl"`
	LeftEmoji  string `json:"leftEmojiCodepoint"`
	RightEmoji string `json:"rightEmojiCodepoint"`
}

var (
	// 内存索引：key 为 "unicode1_unicode2" (从小到大排序)，value 为 URL
	mixCache map[string]string
	once     sync.Once
)

func init() {
	// 加载数据
	loadMetadata()

	control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "合成emoji",
		Help:             "- [emoji][emoji]",
	}).OnMessage(match).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			r := ctx.State["emojimix"].([]rune)
			url := getEmojiURLFromMetadata(r[0], r[1])

			if url != "" {
				ctx.SendChain(message.Image(url))
				return
			}
			logrus.Debugf("[emojimix] metadata 中未找到合成: %x + %x", r[0], r[1])
		})
}

// 核心查询逻辑：不再需要 http 请求，直接查 map
func getEmojiURLFromMetadata(r1, r2 rune) string {
	// r1, r2 本身就是 rune，%x 得到的是 1f42e 和 2601 (不带后缀)
	s1 := fmt.Sprintf("%x", r1)
	s2 := fmt.Sprintf("%x", r2)

	keys := []string{s1, s2}
	sort.Strings(keys)
	cacheKey := keys[0] + "_" + keys[1]

	return mixCache[cacheKey]
}

// 加载本地 metadata.json 并转换为快速索引 map
// 辅助函数：去掉 Unicode 字符串中的变体后缀
func normalize(s string) string {
	s = strings.ReplaceAll(s, "-fe0f", "")
	s = strings.ReplaceAll(s, "-ufe0f", "") // 兼容可能存在的不同前缀
	return s
}
func loadMetadata() {
	once.Do(func() {
		mixCache = make(map[string]string)
		path := filepath.Join("data", "emojimix", "metadata.json")

		file, err := os.ReadFile(path)
		if err != nil {
			logrus.Errorf("[emojimix] 读取 metadata 失败: %v", err)
			return
		}

		var rawData EmojiData
		if err := json.Unmarshal(file, &rawData); err != nil {
			logrus.Errorf("[emojimix] 解析 metadata 失败: %v", err)
			return
		}

		// 遍历外层 Data (例如 "1f42e")
		for rootHex, info := range rawData.Data {
			rootClean := normalize(rootHex)
			
			// 遍历内层 Combinations (例如 "2601-fe0f")
			for comboHex, combos := range info.Combinations {
				comboClean := normalize(comboHex)
				
				// 只要有 URL，我们就根据这两个 Key 生成索引
				if len(combos) > 0 {
					// 排序以确保 A_B 和 B_A 都能搜到
					k := []string{rootClean, comboClean}
					sort.Strings(k)
					key := k[0] + "_" + k[1]
					
					if _, ok := mixCache[key]; !ok {
						mixCache[key] = combos[0].GStaticUrl
					}
				}
			}
		}
		logrus.Infof("[emojimix] 成功加载 %d 条全向索引", len(mixCache))
	})
}
// 匹配逻辑保持不变，但移除了对硬编码 map 的依赖
func match(ctx *zero.Ctx) bool {
	// 获取原始 rune 数组
	var rawRunes []rune
	if len(ctx.Event.Message) == 2 {
		r1 := face2emoji(ctx.Event.Message[0])
		r2 := face2emoji(ctx.Event.Message[1])
		rawRunes = []rune{r1, r2}
	} else {
		rawRunes = []rune(ctx.Event.RawMessage)
	}

	// 【关键修正】：过滤掉所有的 FE0F (Variation Selector-16)
	filtered := make([]rune, 0, len(rawRunes))
	for _, val := range rawRunes {
		if val != 0 && val != 0xFE0F && val != 0xFE0E {
			filtered = append(filtered, val)
		}
	}

	// 过滤后必须刚好是 2 个 emoji
	if len(filtered) == 2 {
		if isEmoji(filtered[0]) && isEmoji(filtered[1]) {
			ctx.State["emojimix"] = filtered
			return true
		}
	}
	return false
}

func isEmoji(r rune) bool {
	// 简单的范围判断，可以根据需要扩充
	return r > 0x2000
}

func face2emoji(face message.Segment) rune {
    if face.Type == "text" {
        r := []rune(face.Data["text"])
        // 同样过滤 text 里的后缀
        if len(r) > 0 && r[0] != 0 {
            return r[0] 
        }
        return 0
    }
    if face.Type != "face" { return 0 }
    id, _ := strconv.Atoi(face.Data["id"])
    if r, ok := message.Emoji[id]; ok {
        return r // 这里拿到的通常是基础码位
    }
    return 0
}