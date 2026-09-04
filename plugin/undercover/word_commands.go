package undercover

import (
	"fmt"
	"strconv"
	"strings"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const wordPageSize = 10

func registerWordCommands() {
	engine.OnRegex(`^添加卧底词\s+([^|｜\s]+)\s*[|｜]\s*([^|｜\s]+)(?:\s*[|｜]\s*([^|｜\s]+))?(?:\s*[|｜]\s*([1-3]))?\s*$`,
		zero.OnlyGroup, zero.AdminPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		matches := ctx.State["regex_matched"].([]string)
		category := "自定义"
		if matches[3] != "" {
			category = matches[3]
		}
		difficulty := 2
		if matches[4] != "" {
			difficulty, _ = strconv.Atoi(matches[4])
		}
		id, err := wordDB.add(matches[1], matches[2], category, difficulty, ctx.Event.UserID)
		if err != nil {
			sendError(ctx, err)
			return
		}
		ctx.SendChain(message.Text("卧底词条添加成功，ID：", id, "，分类：", category, "，难度：", difficulty))
	})

	engine.OnRegex(`^(启用|禁用)卧底词\s+(\d+)$`, zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			matches := ctx.State["regex_matched"].([]string)
			id, err := strconv.ParseInt(matches[2], 10, 64)
			if err != nil {
				sendError(ctx, err)
				return
			}
			enabled := matches[1] == "启用"
			if err := wordDB.setEnabled(id, enabled); err != nil {
				sendError(ctx, err)
				return
			}
			ctx.SendChain(message.Text("已", matches[1], "卧底词条 #", id))
		})

	engine.OnRegex(`^卧底词库(?:\s+(\d+))?$`, zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			matches := ctx.State["regex_matched"].([]string)
			page := 1
			if matches[1] != "" {
				page, _ = strconv.Atoi(matches[1])
			}
			rows, total, err := wordDB.list(page, wordPageSize)
			if err != nil {
				sendError(ctx, err)
				return
			}
			pages := (total + wordPageSize - 1) / wordPageSize
			if pages == 0 {
				pages = 1
			}
			if len(rows) == 0 {
				ctx.SendChain(message.Text("该页没有词条。词库共", total, "条，", pages, "页。"))
				return
			}
			var b strings.Builder
			fmt.Fprintf(&b, "卧底词库 第%d/%d页（共%d条）\n", page, pages, total)
			for _, row := range rows {
				status := "启用"
				if row.Enabled == 0 {
					status = "禁用"
				}
				fmt.Fprintf(&b, "#%d %s / %s｜%s｜难度%d｜%s｜抽取%d次\n",
					row.ID, row.WordA, row.WordB, row.Category, row.Difficulty, status, row.UseCount)
			}
			ctx.SendChain(message.Text(strings.TrimSuffix(b.String(), "\n")))
		})

	engine.OnFullMatch("卧底词库统计", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			stats, total, enabled, err := wordDB.stats()
			if err != nil {
				sendError(ctx, err)
				return
			}
			var b strings.Builder
			fmt.Fprintf(&b, "卧底词库：共%d条，启用%d条，禁用%d条\n", total, enabled, total-enabled)
			for _, stat := range stats {
				fmt.Fprintf(&b, "%s：%d/%d条启用\n", stat.Category, stat.Enabled, stat.Total)
			}
			ctx.SendChain(message.Text(strings.TrimSuffix(b.String(), "\n")))
		})
}
