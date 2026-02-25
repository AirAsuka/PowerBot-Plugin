package emojimix

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// 常用日期列表，按更新频率排列，涵盖了绝大多数合成表情
// var commonDates = []int64{20201001, 20210218, 20210521, 20210831, 20211115, 20220110, 20220224}
var commonDates = []int64{20240206, 20230803, 20230301, 20220224, 20211115, 20210831, 20210521, 20210218, 20201001}

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
	// 1. 核心格式化函数
	// Google 要求：文件夹名和文件名中的 u 后面，4位码要补齐，5位码保持原样
	toGoogleStr := func(r rune, isFileName bool) string {
		h := fmt.Sprintf("%x", r)
		// 如果是 4 位及以下，通常补齐到 4 位（使用 %04x）
		if r < 0x10000 {
			h = fmt.Sprintf("%04x", r)
			// 文件名部分针对特殊符号添加变体选择符
			if isFileName && (r == 0x2665 || r == 0x2b50 || r == 0x263a || r == 0x2764) {
				h += "-ufe0f"
			}
		}
		return h
	}

	// 准备两种顺序的参数
	// trial: {文件夹名, 左文件名, 右文件名}
	trials := [][]string{
		{toGoogleStr(r1, false), toGoogleStr(r1, true), toGoogleStr(r2, true)},
		{toGoogleStr(r2, false), toGoogleStr(r2, true), toGoogleStr(r1, true)},
	}

	client := &http.Client{Timeout: 2 * time.Second}

	for _, date := range commonDates {
		for _, t := range trials {
			// 路径模版：.../日期/u文件夹/u左文件名_u右文件名.png
			testURL := fmt.Sprintf("https://www.gstatic.com/android/keyboard/emojikitchen/%d/u%s/u%s_u%s.png",
				date, t[0], t[1], t[2])

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