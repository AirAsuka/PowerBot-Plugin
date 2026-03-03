package amongus

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/web"
	"github.com/FloatTech/gg"
	"github.com/FloatTech/imgfactory"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/img/text"
	amongusdict "github.com/FloatTech/ZeroBot-Plugin/plugin/amongus/dict"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	queryAPI  = "https://toue.mxyx.club/api/query"
	detailAPI = "https://api.toue.mxyx.club/api/detail/"
)

var (
	// 字典内容已迁移到子包 plugin/amongus/dict
	winConditionText = amongusdict.WinConditionText
	roleText         = amongusdict.RoleText
)

type recentGame struct {
	GameID       string
	StartTime    string
	Duration     string
	PlayerCount  int64
	WinCondition string
}

type paginationInfo struct {
	Page       int64
	PageSize   int64
	Total      int64
	TotalPages int64
}

func init() {
	// 最近n场
	engine.OnRegex(`^最近\s*([0-9]{1,2})\s*场(?:\s+([0-9]{1,4}))?$`, getDB).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			nText := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
			n, err := strconv.Atoi(nText)
			if err != nil || n <= 0 || n > 10 {
				ctx.SendChain(message.Text("参数错误：n必须是1到10之间"))
				return
			}
			// m 控制 page，不传默认 1
			mText := strings.TrimSpace(ctx.State["regex_matched"].([]string)[2])
			page := 1
			if mText != "" {
				m, err := strconv.Atoi(mText)
				if err != nil || m <= 0 {
					ctx.SendChain(message.Text("参数错误：m必须是大于0的整数"))
					return
				}
				page = m
			}

			user, err := database.find(ctx.Event.UserID)
			if err != nil || user.AmongusID == "" {
				ctx.SendChain(message.Text("你还没有录入AmongUs ID，请先使用「录入信息 xxxx」绑定"))
				return
			}

			games, pg, err := queryRecentGames(user.AmongusID, page, n)
			if err != nil {
				ctx.SendChain(message.Text("[amongus] 查询失败: ", err))
				return
			}
			if len(games) == 0 {
				ctx.SendChain(message.Text("未查询到最近对局记录"))
				return
			}
			ctx.SendChain(message.Text(formatRecentGames(user.AmongusID, games, pg)))
		})

	// 游戏详情 <gameId>
	engine.OnRegex(`^游戏详情(?:\s+(.+))?$`, getDB).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gameID := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
			if gameID == "" {
				user, err := database.find(ctx.Event.UserID)
				if err != nil || user.AmongusID == "" {
					ctx.SendChain(message.Text("你还没有录入AmongUs ID，请先使用「录入信息 xxxx」绑定"))
					return
				}

				games, _, err := queryRecentGames(user.AmongusID, 1, 1)
				if err != nil {
					ctx.SendChain(message.Text("[amongus] 获取最近1场失败: ", err))
					return
				}
				if len(games) == 0 || games[0].GameID == "" {
					ctx.SendChain(message.Text("[amongus] 获取最近1场失败: 未找到有效对局ID"))
					return
				}
				gameID = games[0].GameID
			}

			detailResult, err := queryGameDetail(gameID)
			if err != nil {
				ctx.SendChain(message.Text("[amongus] 查询游戏详情失败: ", err))
				return
			}

			// 生成摘要图片（全局信息 + 玩家信息）
			imgBytes, err := renderGameDetailImage(gameID, detailResult)
			if err != nil {
				// 渲染失败则退回文本摘要（不再回传原始JSON）
				ctx.SendChain(message.Text(formatGameDetailSummary(gameID, detailResult)))
				return
			}
			ctx.SendChain(message.ImageBytes(imgBytes))
		})
}

func queryRecentGames(amongusID string, page int, pageSize int) ([]recentGame, paginationInfo, error) {
	encodedID := url.PathEscape(amongusID)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 1
	}
	fullURL := fmt.Sprintf("%s?playerCode=%s&page=%d&pageSize=%d", queryAPI, encodedID, page, pageSize)

	data, err := web.GetData(fullURL)
	if err != nil {
		return nil, paginationInfo{}, err
	}

	result := gjson.ParseBytes(data)
	if !result.Get("success").Bool() {
		return nil, paginationInfo{}, fmt.Errorf(errorMessageFromResult(result, "查询最近对局失败"))
	}

	items := result.Get("data").Array()
	games := make([]recentGame, 0, len(items))
	for _, item := range items {
		games = append(games, recentGame{
			GameID:       item.Get("gameId").String(),
			StartTime:    item.Get("startTime").String(),
			Duration:     item.Get("duration").String(),
			PlayerCount:  item.Get("playerCount").Int(),
			WinCondition: item.Get("winCondition").String(),
		})
	}

	pg := paginationInfo{
		Page:       result.Get("pagination.page").Int(),
		PageSize:   result.Get("pagination.pageSize").Int(),
		Total:      result.Get("pagination.total").Int(),
		TotalPages: result.Get("pagination.totalPages").Int(),
	}
	return games, pg, nil
}

func formatRecentGames(amongusID string, games []recentGame, pg paginationInfo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("玩家ID：%s\n", amongusID))
	sb.WriteString("=================================\n")
	for _, game := range games {
		winText, ok := winConditionText[game.WinCondition]
		if !ok {
			winText = "未知"
		}
		sb.WriteString(fmt.Sprintf("| 游戏ID：%s | 开始时间：%s | 游戏时长：%s | 玩家数量：%d | 胜利信息：%s|\n",
			game.GameID, game.StartTime, game.Duration, game.PlayerCount, winText))
		sb.WriteString("=================================\n")
	}
	// 分页信息（来自 /api/query 的 pagination 字段）
	sb.WriteString(fmt.Sprintf("页数：%d 页面数目：%d，总数：%d，总页数：%d\n",
		pg.Page, pg.PageSize, pg.Total, pg.TotalPages))
	return strings.TrimSpace(sb.String())
}

func queryGameDetail(gameID string) (gjson.Result, error) {
	encodedGameID := url.PathEscape(gameID)
	fullURL := detailAPI + encodedGameID

	data, err := web.GetData(fullURL)
	if err != nil {
		return gjson.Result{}, err
	}

	result := gjson.ParseBytes(data)
	if !result.Get("success").Bool() {
		return gjson.Result{}, fmt.Errorf(errorMessageFromResult(result, "查询游戏详情失败"))
	}
	return result, nil
}

func errorMessageFromResult(result gjson.Result, fallback string) string {
	candidates := []string{
		result.Get("message").String(),
		result.Get("msg").String(),
		result.Get("error").String(),
		result.Get("data.message").String(),
	}
	for _, msg := range candidates {
		if strings.TrimSpace(msg) != "" {
			return msg
		}
	}
	return fallback
}

func mapOrSelf(m map[string]string, key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	if v, ok := m[key]; ok {
		return v
	}
	return key
}

func winText(winCondition string) string {
	if v, ok := winConditionText[winCondition]; ok {
		return v
	}
	return "未知"
}

func parseGameTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0001-01-01T00:00:00" {
		return time.Time{}, false
	}
	// API 示例为无时区的 RFC3339-like
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		// 兜底尝试 RFC3339
		t2, err2 := time.Parse(time.RFC3339, s)
		if err2 != nil {
			return time.Time{}, false
		}
		return t2, true
	}
	return t, true
}

func parseHMSDurationToSeconds(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	if h < 0 || m < 0 || sec < 0 {
		return 0, false
	}
	return int64(h*3600 + m*60 + sec), true
}

func formatMMSS(totalSeconds int64) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	min := totalSeconds / 60
	sec := totalSeconds % 60
	return fmt.Sprintf("%02d分%02d秒", min, sec)
}

func formatDeathInfo(reason, killedBy string, isDead bool) string {
	if !isDead {
		return "存活"
	}
	reason = strings.TrimSpace(reason)
	killedBy = strings.TrimSpace(killedBy)

	switch reason {
	case "", "Null", "None":
		return "无"
	case "Alive":
		return "存活"
	case "Suicide":
		return "自杀"
	case "Canceled":
		return "房主结束游戏"
	case "LawyerSuicide":
		return "辩护失败"
	case "LoverSuicide":
		return "殉情"
	case "Loneliness":
		return "精力衰竭"
	case "Shift":
		return "交换失败"
	case "GuessFail":
		return "猜测错误"
	case "FakeSK":
		return "招募失败"
	case "WrongVerdict":
		return "裁决错误"
	case "SheriffSuicide":
		return "警长走火"
	case "AvengerFail":
		return "复仇失败"
	case "BombVictim":
		return "恐袭"
	case "Exile":
		return "被驱逐"
	case "Disconnect":
		return "断开连接"
	case "HostKill":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 制裁", killedBy)
		}
		return "被房主制裁"
	case "Kill":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 击杀", killedBy)
		}
		return "被击杀"
	case "GuessSuccess":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 猜测", killedBy)
		}
		return "被猜测"
	case "WitchExile":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 咒杀", killedBy)
		}
		return "被咒杀"
	case "Bomb":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 恐袭", killedBy)
		}
		return "被恐袭"
	case "LoveStolen":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 夺走了爱人", killedBy)
		}
		return "爱人被夺"
	case "Arson":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 烧死", killedBy)
		}
		return "被烧死"
	case "SheriffKill":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 出警", killedBy)
		}
		return "被出警"
	case "SheriffMisfire":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 误杀", killedBy)
		}
		return "被误杀"
	case "Eaten":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 吞食", killedBy)
		}
		return "被吞食"
	case "Jailed":
		if killedBy != "" && killedBy != "null" {
			return fmt.Sprintf("被 %s 处决", killedBy)
		}
		return "被处决"
	default:
		// 未知死亡原因：保留原始值便于排查
		return reason
	}
}

func formatGameDetailSummary(gameID string, detail gjson.Result) string {
	global := detail.Get("data.Global")
	players := detail.Get("data.Players").Array()

	duration := global.Get("Duration").String()
	startTimeStr := global.Get("StartTime").String()
	endTimeStr := global.Get("EndTime").String()
	startTime, okStart := parseGameTime(startTimeStr)
	endTime, okEnd := parseGameTime(endTimeStr)

	// 兜底：如果 EndTime 解析失败，尝试用 Duration 推算
	if okStart && !okEnd {
		if sec, ok := parseHMSDurationToSeconds(duration); ok {
			endTime = startTime.Add(time.Duration(sec) * time.Second)
			okEnd = true
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("游戏详情 - %s\n", gameID))
	sb.WriteString("全局信息\n")
	sb.WriteString(fmt.Sprintf("游戏版本\t%s\t房主\t%s\t房间代码\t%s\n",
		global.Get("GameVersion").String(),
		global.Get("HostPlayer").String(),
		global.Get("RoomCode").String(),
	))
	sb.WriteString(fmt.Sprintf("开始时间\t%s\t结束时间\t%s\t游戏时长\t%s\n",
		startTimeStr, endTimeStr, duration,
	))
	sb.WriteString(fmt.Sprintf("胜利条件\t%s\t玩家数量\t%d\t房主代码\t%s\n",
		winText(global.Get("WinCondition").String()),
		global.Get("PlayerCount").Int(),
		global.Get("HostCode").String(),
	))
	sb.WriteString("\n玩家信息\n")
	sb.WriteString("玩家 | 主职业 | 职业详情 | 职业历史 | 胜利 | 击杀 | 任务 | 存活时间 | 死亡信息\n")
	sb.WriteString("--------------------------------------------------------------------------------\n")

	// 全局时长（秒）备用
	globalSec, okGlobalSec := parseHMSDurationToSeconds(duration)

	for _, p := range players {
		playerName := p.Get("PlayerName").String()
		mainRole := mapOrSelf(roleText, p.Get("RoleInfo.MainRole").String())

		// RoleDetails / RoleHistories 可能为空数组
		detailsArr := p.Get("RoleInfo.RoleDetails").Array()
		details := make([]string, 0, len(detailsArr))
		for _, d := range detailsArr {
			details = append(details, mapOrSelf(roleText, d.String()))
		}

		histArr := p.Get("RoleInfo.RoleHistories").Array()
		hists := make([]string, 0, len(histArr))
		for _, h := range histArr {
			hists = append(hists, mapOrSelf(roleText, h.String()))
		}

		isWinner := p.Get("GameplayStats.IsWinner").Bool()
		isDead := p.Get("GameplayStats.IsDead").Bool()
		killCount := p.Get("GameplayStats.KillCount").Int()
		if killCount > 100 { # 律师击杀有144
			killCount = 0
		}
		completed := p.Get("GameplayStats.Tasks.Completed").Int()
		total := p.Get("GameplayStats.Tasks.Total").Int()
		deathReason := p.Get("GameplayStats.DeathReason").String()
		killedBy := p.Get("GameplayStats.KilledBy").String()

		deathTimeStr := p.Get("GameplayStats.DeathTime").String()
		deathTime, okDeath := parseGameTime(deathTimeStr)

		aliveSeconds := int64(0)
		switch {
		case okStart && okDeath && isDead:
			aliveSeconds = int64(deathTime.Sub(startTime).Seconds())
		case okStart && okEnd:
			aliveSeconds = int64(endTime.Sub(startTime).Seconds())
		case okGlobalSec:
			aliveSeconds = globalSec
		default:
			aliveSeconds = 0
		}

		winCN := "否"
		if isWinner {
			winCN = "是"
		}

		deathInfo := formatDeathInfo(deathReason, killedBy, isDead)
		detailText := strings.Join(details, ",")
		histText := strings.Join(hists, ",")
		taskText := fmt.Sprintf("%d/%d", completed, total)
		timeText := formatMMSS(aliveSeconds)

		sb.WriteString(fmt.Sprintf("%s | %s | %s | %s | %s | %d | %s | %s | %s\n",
			playerName, mainRole, detailText, histText, winCN, killCount, taskText, timeText, deathInfo))
	}

	return strings.TrimSpace(sb.String())
}

func renderGameDetailImage(gameID string, detail gjson.Result) ([]byte, error) {
	global := detail.Get("data.Global")
	players := detail.Get("data.Players").Array()

	if !global.Exists() {
		return nil, errors.New("缺少 data.Global")
	}

	// 解析时间用于“存活时间”
	duration := global.Get("Duration").String()
	startTimeStr := global.Get("StartTime").String()
	endTimeStr := global.Get("EndTime").String()
	startTime, okStart := parseGameTime(startTimeStr)
	endTime, okEnd := parseGameTime(endTimeStr)
	if okStart && !okEnd {
		if sec, ok := parseHMSDurationToSeconds(duration); ok {
			endTime = startTime.Add(time.Duration(sec) * time.Second)
			okEnd = true
		}
	}
	globalSec, okGlobalSec := parseHMSDurationToSeconds(duration)

	// 画布布局
	const (
		padding       = 40.0
		titleSize     = 44.0
		subTitleSize  = 32.0
		headerSize    = 24.0
		bodySize      = 22.0
		rowH          = 62.0 // 单行/双行都留足空间，避免贴边
		globalRowH    = 68.0 // 全局信息每格允许轻微换行
		cardRadius    = 18.0
		lineW         = 2.0
		tableHeaderH  = 56.0
		globalTopPad  = 68.0 // 全局卡片标题区到网格起始的距离
		tableTopPad   = 64.0 // 玩家卡片标题区到表头起始的距离
		cardBottomPad = 28.0 // 卡片底部留白，防止文字踩边框
	)

	// 列宽（按你给的字段顺序）
	colW := []float64{
		220, // 玩家
		120, // 主职业
		220, // 职业详情
		220, // 职业历史
		70,  // 胜利
		70,  // 击杀
		110, // 任务
		120, // 存活时间
		260, // 死亡信息
	}
	tableW := 0.0
	for _, w := range colW {
		tableW += w
	}

	// 计算整体高度（不分页：直接按玩家数拉长）
	titleH := 70.0
	sectionGap := 18.0
	// 全局卡片高度 = 标题区 + 3行网格 + 底部留白
	globalBlockH := globalTopPad + globalRowH*3 + cardBottomPad
	// 玩家卡片高度 = 标题区 + 表头 + N行 + 底部留白
	tableCardH := tableTopPad + tableHeaderH + rowH*float64(len(players)) + cardBottomPad
	canvasW := padding*2 + tableW
	canvasH := padding + titleH + sectionGap + globalBlockH + sectionGap + tableCardH + padding

	c := gg.NewContext(int(canvasW), int(canvasH))
	// 背景
	c.SetRGB255(245, 247, 250)
	c.Clear()

	// 字体
	boldFont, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
	if err != nil {
		return nil, err
	}
	regularFont, err := file.GetLazyData(text.FontFile, control.Md5File, true)
	if err != nil {
		return nil, err
	}

	// 标题卡片
	cardX, cardY := padding, padding
	cardW := canvasW - padding*2

	c.SetRGBA255(255, 255, 255, 255)
	c.DrawRoundedRectangle(cardX, cardY, cardW, titleH, cardRadius)
	c.Fill()
	c.SetRGBA255(0, 0, 0, 18)
	c.SetLineWidth(lineW)
	c.DrawRoundedRectangle(cardX, cardY, cardW, titleH, cardRadius)
	c.Stroke()

	if err = c.ParseFontFace(boldFont, titleSize); err != nil {
		return nil, err
	}
	c.SetRGB255(30, 41, 59)
	c.DrawStringAnchored(fmt.Sprintf("游戏详情 - %s", gameID), cardX+20, cardY+titleH/2, 0, 0.5)

	// 全局信息卡片
	globalX := padding
	globalY := cardY + titleH + sectionGap
	c.SetRGBA255(255, 255, 255, 255)
	c.DrawRoundedRectangle(globalX, globalY, cardW, globalBlockH, cardRadius)
	c.Fill()
	c.SetRGBA255(0, 0, 0, 18)
	c.SetLineWidth(lineW)
	c.DrawRoundedRectangle(globalX, globalY, cardW, globalBlockH, cardRadius)
	c.Stroke()

	if err = c.ParseFontFace(boldFont, subTitleSize); err != nil {
		return nil, err
	}
	c.SetRGB255(15, 23, 42)
	c.DrawStringAnchored("全局信息", globalX+20, globalY+36, 0, 0.5)

	// 全局网格：三行三列（每行3组 key/value）
	playerCount := fmt.Sprintf("%d", global.Get("PlayerCount").Int())
	rows := [][][2]string{
		{
			{"游戏版本", global.Get("GameVersion").String()},
			{"房主", global.Get("HostPlayer").String()},
			{"房间代码", global.Get("RoomCode").String()},
		},
		{
			{"开始时间", startTimeStr},
			{"结束时间", endTimeStr},
			{"游戏时长", duration},
		},
		{
			{"胜利条件", winText(global.Get("WinCondition").String())},
			{"玩家数量", playerCount},
			{"房主代码", global.Get("HostCode").String()},
		},
	}

	gridTop := globalY + globalTopPad
	gridLeft := globalX + 20
	gridW := cardW - 40
	cellW := gridW / 3

	drawKV := func(x, y float64, k, v string) error {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if err := c.ParseFontFace(boldFont, headerSize); err != nil {
			return err
		}
		c.SetRGB255(71, 85, 105)
		c.DrawStringAnchored(k, x, y, 0, 0.5)
		keyW, _ := c.MeasureString(k)
		if err := c.ParseFontFace(regularFont, bodySize); err != nil {
			return err
		}
		c.SetRGB255(15, 23, 42)
		// value 起始位置根据 key 宽度动态偏移，避免 key/value 文字重叠
		const gap = 16.0
		valueX := x + keyW + gap
		right := x + cellW - 10 // 右侧留白
		if valueX > right {
			valueX = right
		}
		maxW := right - valueX
		if maxW < 10 {
			return nil
		}
		c.DrawStringWrapped(v, valueX, y, 0, 0.5, maxW, 1.35, gg.AlignLeft)
		return nil
	}

	// 画三行，每行3格
	for r := 0; r < len(rows); r++ {
		row := rows[r]
		for i := 0; i < 3 && i < len(row); i++ {
			if err := drawKV(gridLeft+cellW*float64(i), gridTop+globalRowH*(float64(r)+0.5), row[i][0], row[i][1]); err != nil {
				return nil, err
			}
		}
	}

	// 玩家信息卡片
	tableX := padding
	tableY := globalY + globalBlockH + sectionGap
	c.SetRGBA255(255, 255, 255, 255)
	c.DrawRoundedRectangle(tableX, tableY, cardW, tableCardH, cardRadius)
	c.Fill()
	c.SetRGBA255(0, 0, 0, 18)
	c.SetLineWidth(lineW)
	c.DrawRoundedRectangle(tableX, tableY, cardW, tableCardH, cardRadius)
	c.Stroke()

	if err = c.ParseFontFace(boldFont, subTitleSize); err != nil {
		return nil, err
	}
	c.SetRGB255(15, 23, 42)
	c.DrawStringAnchored("玩家信息", tableX+20, tableY+36, 0, 0.5)

	// 表头背景
	headerX := tableX + 20
	headerY := tableY + tableTopPad
	c.SetRGBA255(15, 23, 42, 8)
	c.DrawRoundedRectangle(headerX, headerY, tableW, tableHeaderH, 12)
	c.Fill()

	headers := []string{"玩家", "主职业", "职业详情", "职业历史", "胜利", "击杀", "任务", "存活时间", "死亡信息"}
	if err = c.ParseFontFace(boldFont, headerSize); err != nil {
		return nil, err
	}
	c.SetRGB255(51, 65, 85)
	x := headerX
	for i, h := range headers {
		c.DrawStringAnchored(h, x+10, headerY+tableHeaderH/2, 0, 0.5)
		x += colW[i]
	}

	// 表格行
	if err = c.ParseFontFace(regularFont, bodySize); err != nil {
		return nil, err
	}
	rowStartY := headerY + tableHeaderH
	for i, p := range players {
		y := rowStartY + rowH*float64(i)
		// 斑马纹
		if i%2 == 0 {
			c.SetRGBA255(148, 163, 184, 10)
			c.DrawRectangle(headerX, y, tableW, rowH)
			c.Fill()
		}
		// 分隔线
		c.SetRGBA255(148, 163, 184, 35)
		c.SetLineWidth(1)
		c.DrawLine(headerX, y+rowH, headerX+tableW, y+rowH)
		c.Stroke()

		playerName := p.Get("PlayerName").String()
		mainRole := mapOrSelf(roleText, p.Get("RoleInfo.MainRole").String())
		detailsArr := p.Get("RoleInfo.RoleDetails").Array()
		details := make([]string, 0, len(detailsArr))
		for _, d := range detailsArr {
			details = append(details, mapOrSelf(roleText, d.String()))
		}
		histArr := p.Get("RoleInfo.RoleHistories").Array()
		hists := make([]string, 0, len(histArr))
		for _, h := range histArr {
			hists = append(hists, mapOrSelf(roleText, h.String()))
		}
		isWinner := p.Get("GameplayStats.IsWinner").Bool()
		isDead := p.Get("GameplayStats.IsDead").Bool()
		killCount := p.Get("GameplayStats.KillCount").Int()
		if killCount > 100 { # 律师击杀有144
			killCount = 0
		}
		completed := p.Get("GameplayStats.Tasks.Completed").Int()
		total := p.Get("GameplayStats.Tasks.Total").Int()
		deathReason := p.Get("GameplayStats.DeathReason").String()
		killedBy := p.Get("GameplayStats.KilledBy").String()
		deathTimeStr := p.Get("GameplayStats.DeathTime").String()
		deathTime, okDeath := parseGameTime(deathTimeStr)

		aliveSeconds := int64(0)
		switch {
		case okStart && okDeath && isDead:
			aliveSeconds = int64(deathTime.Sub(startTime).Seconds())
		case okStart && okEnd:
			aliveSeconds = int64(endTime.Sub(startTime).Seconds())
		case okGlobalSec:
			aliveSeconds = globalSec
		default:
			aliveSeconds = 0
		}

		winCN := "否"
		if isWinner {
			winCN = "是"
		}
		taskText := fmt.Sprintf("%d/%d", completed, total)
		timeText := formatMMSS(aliveSeconds)
		deathInfo := formatDeathInfo(deathReason, killedBy, isDead)

		cells := []string{
			playerName,
			mainRole,
			strings.Join(details, ","),
			strings.Join(hists, ","),
			winCN,
			fmt.Sprintf("%d", killCount),
			taskText,
			timeText,
			deathInfo,
		}

		x = headerX
		for ci, cell := range cells {
			c.SetRGB255(15, 23, 42)
			if ci == 4 { // 胜利列颜色强调
				if winCN == "是" {
					c.SetRGB255(22, 163, 74)
				} else {
					c.SetRGB255(239, 68, 68)
				}
			}

			// 文本绘制：左对齐，超长自动换行（最多2行）
			maxW := colW[ci] - 20
			align := gg.AlignLeft
			anchorX := 0.0
			drawX := x + 10
			if ci == 4 || ci == 5 || ci == 6 || ci == 7 { // 小字段居中
				align = gg.AlignCenter
				anchorX = 0.5
				drawX = x + colW[ci]/2
			}

			c.DrawStringWrapped(cell, drawX, y+rowH/2, anchorX, 0.5, maxW, 1.25, align)
			x += colW[ci]
		}
	}

	return imgfactory.ToBytes(c.Image())
}
