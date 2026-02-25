// // Package emojimix 合成emoji
// package emojimix

// import (
// 	"fmt"
// 	"net/http"
// 	"strconv"

// 	ctrl "github.com/FloatTech/zbpctrl"
// 	"github.com/FloatTech/zbputils/control"
// 	"github.com/FloatTech/zbputils/ctxext"
// 	"github.com/sirupsen/logrus"
// 	zero "github.com/wdvxdr1123/ZeroBot"
// 	"github.com/wdvxdr1123/ZeroBot/message"
// )

// const bed = "https://www.gstatic.com/android/keyboard/emojikitchen/%d/u%x/u%x_u%x.png"

// func init() {
// 	control.AutoRegister(&ctrl.Options[*zero.Ctx]{
// 		DisableOnDefault: false,
// 		Brief:            "合成emoji",
// 		Help:             "- [emoji][emoji]",
// 	}).OnMessage(match).SetBlock(true).Limit(ctxext.LimitByUser).
// 		Handle(func(ctx *zero.Ctx) {
// 			r := ctx.State["emojimix"].([]rune)
// 			logrus.Debugln("[emojimix] match:", r)
// 			r1, r2 := r[0], r[1]
// 			u1 := fmt.Sprintf(bed, emojis[r1], r1, r1, r2)
// 			u2 := fmt.Sprintf(bed, emojis[r2], r2, r2, r1)
// 			logrus.Debugln("[emojimix] u1:", u1)
// 			logrus.Debugln("[emojimix] u2:", u2)
// 			resp1, err := http.Head(u1)
// 			if err == nil {
// 				resp1.Body.Close()
// 				if resp1.StatusCode == http.StatusOK {
// 					ctx.SendChain(message.Image(u1))
// 					return
// 				}
// 			}
// 			resp2, err := http.Head(u2)
// 			if err == nil {
// 				resp2.Body.Close()
// 				if resp2.StatusCode == http.StatusOK {
// 					ctx.SendChain(message.Image(u2))
// 					return
// 				}
// 			}
// 		})
// }

// func match(ctx *zero.Ctx) bool {
// 	logrus.Debugln("[emojimix] msg:", ctx.Event.Message)
// 	if len(ctx.Event.Message) == 2 {
// 		r1 := face2emoji(ctx.Event.Message[0])
// 		if _, ok := emojis[r1]; !ok {
// 			return false
// 		}
// 		r2 := face2emoji(ctx.Event.Message[1])
// 		if _, ok := emojis[r2]; !ok {
// 			return false
// 		}
// 		ctx.State["emojimix"] = []rune{r1, r2}
// 		return true
// 	}

// 	r := []rune(ctx.Event.RawMessage)
// 	logrus.Debugln("[emojimix] raw msg:", ctx.Event.RawMessage)
// 	if len(r) == 2 {
// 		if _, ok := emojis[r[0]]; !ok {
// 			return false
// 		}
// 		if _, ok := emojis[r[1]]; !ok {
// 			return false
// 		}
// 		ctx.State["emojimix"] = r
// 		return true
// 	}
// 	return false
// }

// func face2emoji(face message.Segment) rune {
// 	if face.Type == "text" {
// 		r := []rune(face.Data["text"])
// 		if len(r) != 1 {
// 			return 0
// 		}
// 		return r[0]
// 	}
// 	if face.Type != "face" {
// 		return 0
// 	}
// 	id, err := strconv.Atoi(face.Data["id"])
// 	if err != nil {
// 		return 0
// 	}
// 	if r, ok := message.Emoji[id]; ok {
// 		return r
// 	}
// 	return 0
// }

package emojimix

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// Metadata 结构体用于解析 Google 的 emoji 映射
type EmojiMetadata struct {
	Emoji      string   `json:"emoji"`
	CodePoint  string   `json:"codePoint"`
	Date       string   `json:"date"`
	Combinations []struct {
		LeftEmoji  string `json:"leftEmoji"`
		RightEmoji string `json:"rightEmoji"`
		Date       string `json:"date"`
	} `json:"combinations"`
}

var (
	// 内存缓存：Key 为十六进制码点，Value 为对应的日期
	emojiDateMap map[string]string
	once         sync.Once
)

// InitMetadata 从远程或本地加载最新的 Emoji 数据
// 建议在程序启动时调用一次
func InitMetadata() error {
	// 这个 URL 包含了目前已知的所有组合和它们的日期
	resp, err := http.Get("https://raw.githubusercontent.com/xsalazar/emoji-kitchen-backend/main/scripts/output/metadata.json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var data struct {
		KnownEmojis map[string]EmojiMetadata `json:"knownEmojis"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	tempMap := make(map[string]string)
	for code, meta := range data.KnownEmojis {
		tempMap[code] = meta.Date
	}
	emojiDateMap = tempMap
	return nil
}

// GetEmojiMixURL 构建通用的 URL
func GetEmojiMixURL(r1, r2 rune) string {
	u1 := fmt.Sprintf("%x", r1)
	u2 := fmt.Sprintf("%x", r2)

	// 尝试从 map 中获取日期，如果没有则尝试默认日期
	date1, ok1 := emojiDateMap[u1]
	if !ok1 {
		date1 = "20201001" // 默认保底日期
	}

	// 构造 Google 的静态资源 URL
	// 格式通常为: .../date/u{r1}/u{r1}_u{r2}.png
	return fmt.Sprintf("https://www.gstatic.com/android/keyboard/emojikitchen/%s/u%s/u%s_u%s.png", date1, u1, u1, u2)
}

// HandleEmojiMix 处理逻辑
func (m *Mixer) Handle(r1, r2 rune) string {
	once.Do(func() {
		_ = InitMetadata() // 懒加载
	})

	// 尝试两种组合（Google 的存储逻辑不固定，有时候是 r1_r2，有时候是 r2_r1）
	url1 := GetEmojiMixURL(r1, r2)
	if checkURL(url1) {
		return url1
	}

	url2 := GetEmojiMixURL(r2, r1)
	if checkURL(url2) {
		return url2
	}

	return ""
}

func checkURL(url string) bool {
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}