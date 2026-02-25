package emojimix

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"strconv"

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
	// 1. 将 rune 转为 google 格式的 hex 字符串
	s1 := fmt.Sprintf("%x", r1)
	s2 := fmt.Sprintf("%x", r2)

	// 2. 排序，确保 Key 唯一性 (a_b 和 b_a 指向同一个结果)
	keys := []string{s1, s2}
	sort.Strings(keys)
	cacheKey := keys[0] + "_" + keys[1]

	return mixCache[cacheKey]
}

// 加载本地 metadata.json 并转换为快速索引 map
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

		// 扁平化数据到 mixCache 中
		for _, info := range rawData.Data {
			for _, combos := range info.Combinations {
				for _, c := range combos {
					// 同样进行排序以保证存入的 key 格式一致
					k := []string{c.LeftEmoji, c.RightEmoji}
					sort.Strings(k)
					key := k[0] + "_" + k[1]
					// 只存储最新的（通常列表第一个就是最新的，或者根据 isLatest 过滤）
					if _, ok := mixCache[key]; !ok {
						mixCache[key] = c.GStaticUrl
					}
				}
			}
		}
		logrus.Infof("[emojimix] 成功加载 %d 条合成索引", len(mixCache))
	})
}
// 匹配逻辑保持不变，但移除了对硬编码 map 的依赖
func match(ctx *zero.Ctx) bool {
	var r []rune
	if len(ctx.Event.Message) == 2 {
		r1 := face2emoji(ctx.Event.Message[0])
		r2 := face2emoji(ctx.Event.Message[1])
		if r1 != 0 && r2 != 0 {
			r = []rune{r1, r2}
		}
	} else {
		tempR := []rune(ctx.Event.RawMessage)
		if len(tempR) == 2 {
			r = tempR
		}
	}

	if len(r) == 2 {
		// 这里简单判断是否在常见的 Emoji Unicode 范围内
		// 避免非表情字符触发大量网络请求
		if isEmoji(r[0]) && isEmoji(r[1]) {
			ctx.State["emojimix"] = r
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
        if len(r) != 1 { return 0 }
        return r[0]
    }
    if face.Type != "face" { return 0 }
    id, _ := strconv.Atoi(face.Data["id"])
    if r, ok := message.Emoji[id]; ok {
        return r
    }
    return 0
}