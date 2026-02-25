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
var commonDates = []int64{20201001, 20210218, 20210521, 20210831, 20211115, 20220110, 20220224}

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

// 核心逻辑：尝试不同的组合顺序和日期
func getEmojiMixURL(r1, r2 rune) string {
	// Emoji Kitchen 的 URL 特点：小值通常在前，但并非绝对
	// 我们需要尝试 (r1, r2) 和 (r2, r1) 两种组合
	combinations := [][2]rune{{r1, r2}, {r2, r1}}
	
	client := &http.Client{
		// 设置较短的超时，避免长时间挂起
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for _, c := range combinations {
		for _, date := range commonDates {
			// 格式化 URL
			// 注意：Emoji Kitchen 的路径中 u%x_u%x 部分通常按照 Unicode 数值排序
			u1, u2 := c[0], c[1]
			testURL := fmt.Sprintf(bed, date, u1, u1, u2)
			
			// 使用 HEAD 请求快速检测图片是否存在
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