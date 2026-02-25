package emojimix

import (
	"fmt"
	"net/http"
	"strconv"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// 常用日期列表，按更新频率排列，涵盖了绝大多数合成表情
// var commonDates = []int64{20201001, 20210218, 20210521, 20210831, 20211115, 20220110, 20220224}
var commonDates = []int64{20230803, 20230301, 20220224, 20211115, 20210831, 20210521, 20210218, 20201001}

const bed = "https://www.gstatic.com/android/keyboard/emojikitchen/%d/u%x/u%x_u%x.png"

func init() {
	control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "合成emoji",
		Help:             "- [emoji][emoji]",
	}).OnMessage(match).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			r := ctx.State["emojimix"].([]rune)
			r1, r2 := r[0], r[1]
			
			// 尝试合成
			url := getEmojiMixURL(r1, r2)
			if url != "" {
				ctx.SendChain(message.Image(url))
				return
			}
			
			// 如果没找到，可以保持沉默或者反馈给用户
			logrus.Debugf("[emojimix] failed to mix: %x + %x", r1, r2)
		})
}
func getEmojiMixURL(r1, r2 rune) string {
    // 1. 专门处理需要 -ufe0f 后缀的特殊表情
    // Emoji Kitchen 要求这些字符在文件名部分必须带后缀
    formatHex := func(r rune) string {
        h := fmt.Sprintf("%x", r)
        // 常见需要加 -ufe0f 的 Unicode 范围或特定字符
        // 2665 = ❤️, 2b50 = ⭐, 263a = ☺️ 等
        if r == 0x2665 || r == 0x2b50 || r == 0x263a || r == 0x2764 {
            return h + "-ufe0f"
        }
        return h
    }

    s1 := formatHex(r1)
    s2 := formatHex(r2)

    // 2. 构造尝试序列
    // Google 的 URL 结构：.../date/u{r1}/u{r1}_u{s2}.png
    // 注意：文件夹名通常只用基础码(%x)，文件名部分可能包含后缀(%s)
    type trial struct {
        folder rune
        left   rune
        right  string
    }

    // 尝试两种组合顺序：r1+r2 和 r2+r1
    trials := []trial{
        {r1, r1, s2},
        {r2, r2, s1},
    }

    client := &http.Client{
        Timeout: 2 * time.Second, // 设置超时防止卡死
    }

    for _, date := range commonDates {
        for _, t := range trials {
            // 注意这里最后的 %s，因为 s2 可能带 -ufe0f
            testURL := fmt.Sprintf("https://www.gstatic.com/android/keyboard/emojikitchen/%d/u%x/u%x_u%s.png",
                date, t.folder, t.left, t.right)

            resp, err := client.Head(testURL)
            if err == nil {
                resp.Body.Close()
                if resp.StatusCode == http.StatusOK {
                    return testURL
                }
            }
        }
    }
    return ""
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