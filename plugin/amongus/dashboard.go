package amongus

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/web"
	"github.com/FloatTech/gg"
	"github.com/FloatTech/imgfactory"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/img/text"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const rankingsAPI = "https://api.toue.mxyx.club/api/rankings/"

// maxRankingRows 每个排行榜最大展示行数
const maxRankingRows = 10

// rankingEntry 排行榜条目，字段与 TOUE-Web /api/rankings/<player_code> 响应保持一致
type rankingEntry struct {
	Name      string
	Count     int64
	Total     int64
	Deaths    int64
	DeathRate float64
}

// playerRankings 个人排行数据
type playerRankings struct {
	TopKillers []rankingEntry // 最常被谁杀
	TopVictims []rankingEntry // 击杀最多的玩家
	DeathRoles []rankingEntry // 死亡率最高的职业
}

func init() {
	// 个人中心
	engine.OnFullMatch("个人中心", getDB).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			user, err := database.find(ctx.Event.UserID)
			if err != nil || user.AmongusID == "" {
				ctx.SendChain(message.Text("你还没有录入AmongUs ID，请先使用「录入信息 xxxx」绑定"))
				return
			}
			// 战绩总览复用 /api/profile/<player_code>
			profile, err := queryPlayerProfile(user.AmongusID)
			if err != nil {
				ctx.SendChain(message.Text("[amongus] 请求失败: ", err))
				return
			}
			// 排行数据来自 /api/rankings/<player_code>，失败时对应栏目显示暂无数据
			rankings, _ := queryPlayerRankings(user.AmongusID)

			imgBytes, err := renderPersonalCenterImage(user.AmongusID, profile, rankings)
			if err != nil {
				ctx.SendChain(message.Text(formatPersonalCenterSummary(user.AmongusID, profile, rankings)))
				return
			}
			ctx.SendChain(message.ImageBytes(imgBytes))
		})
}

// queryPlayerRankings 请求 /api/rankings/<player_code> 并解析个人排行
func queryPlayerRankings(amongusID string) (*playerRankings, error) {
	fullURL := rankingsAPI + url.PathEscape(amongusID)
	data, err := web.GetData(fullURL)
	if err != nil {
		return nil, err
	}
	result := gjson.ParseBytes(data)
	if !result.Get("success").Bool() {
		return nil, fmt.Errorf(errorMessageFromResult(result, "查询排行失败"))
	}

	rankings := &playerRankings{}
	parseEntries := func(path string) []rankingEntry {
		items := result.Get(path).Array()
		entries := make([]rankingEntry, 0, len(items))
		for _, item := range items {
			entries = append(entries, rankingEntry{
				Name:      item.Get("name").String(),
				Count:     item.Get("count").Int(),
				Total:     item.Get("total").Int(),
				Deaths:    item.Get("deaths").Int(),
				DeathRate: item.Get("deathRate").Float(),
			})
		}
		return entries
	}

	rankings.TopKillers = parseEntries("data.topKillers")
	rankings.TopVictims = parseEntries("data.topVictims")
	rankings.DeathRoles = mergeDeathRolesByDisplayName(parseEntries("data.deathRoles"))
	return rankings, nil
}

// mergeDeathRolesByDisplayName 按职业中文名去重合并并重算死亡率（与 Web 端 UserDashboard 逻辑一致）
func mergeDeathRolesByDisplayName(entries []rankingEntry) []rankingEntry {
	merged := make(map[string]*rankingEntry, len(entries))
	order := make([]string, 0, len(entries))
	for _, e := range entries {
		name := mapOrSelf(roleText, e.Name)
		if m, ok := merged[name]; ok {
			m.Total += e.Total
			m.Deaths += e.Deaths
		} else {
			entry := e
			entry.Name = name
			merged[name] = &entry
			order = append(order, name)
		}
	}
	result := make([]rankingEntry, 0, len(order))
	for _, name := range order {
		m := merged[name]
		if m.Total > 0 {
			m.DeathRate = float64(m.Deaths) / float64(m.Total) * 100
		} else {
			m.DeathRate = 0
		}
		result = append(result, *m)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].DeathRate > result[j].DeathRate
	})
	return result
}

// formatPersonalCenterSummary 个人中心文本摘要（图片渲染失败时的兜底）
func formatPersonalCenterSummary(amongusID string, p *profileData, r *playerRankings) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("══ 个人中心 - %s ══\n\n", amongusID))

	sb.WriteString("【战绩总览】\n")
	sb.WriteString(fmt.Sprintf("  总场次:      %d\n", p.Total.TotalMatches))
	sb.WriteString(fmt.Sprintf("  总胜率:      %.2f%%\n", p.Total.WinRate))
	sb.WriteString(fmt.Sprintf("  平均击杀:    %.2f\n", p.Total.AverageKills))
	sb.WriteString(fmt.Sprintf("  任务完成度:  %.2f%%\n", p.Total.TaskCompletionRate))
	sb.WriteString(fmt.Sprintf("  全勤任务率:  %.2f%%\n\n", p.Total.CompletedAllTasksRate))

	writeRanking := func(title string, entries []rankingEntry, isDeathRate bool) {
		sb.WriteString("【" + title + "】\n")
		if len(entries) == 0 {
			sb.WriteString("  暂无数据\n")
		}
		for i, e := range entries {
			if isDeathRate {
				sb.WriteString(fmt.Sprintf("  %d. %s 死亡率 %.2f%% (%d/%d)\n", i+1, e.Name, e.DeathRate, e.Deaths, e.Total))
			} else {
				sb.WriteString(fmt.Sprintf("  %d. %s %d 次\n", i+1, e.Name, e.Count))
			}
		}
		sb.WriteString("\n")
	}
	if r == nil {
		r = &playerRankings{}
	}
	writeRanking("击杀排行", r.TopVictims, false)
	writeRanking("被击杀排行", r.TopKillers, false)
	writeRanking("死亡率排行", r.DeathRoles, true)

	return strings.TrimSpace(sb.String())
}

// renderPersonalCenterImage 渲染个人中心图片：战绩总览 + 击杀/被击杀/死亡率排行
func renderPersonalCenterImage(amongusID string, p *profileData, r *playerRankings) ([]byte, error) {
	const (
		padding       = 40.0
		titleSize     = 44.0
		subTitleSize  = 32.0
		headerSize    = 24.0
		bodySize      = 22.0
		smallSize     = 18.0
		rankRowH      = 44.0
		statValueSize = 34.0
		statLabelSize = 20.0
		cardRadius    = 18.0
		lineW         = 2.0
		sectionTopPad = 68.0 // 卡片标题区到内容的距离
		cardBottomPad = 28.0 // 卡片底部留白，防止文字踩边框
		rankColW      = 320.0
		rankColGap    = 16.0
	)

	if r == nil {
		r = &playerRankings{}
	}

	cardW := rankColW*3 + rankColGap*2 + 40
	sectionGap := 18.0
	titleH := 70.0
	overviewRowH := 120.0
	overviewCardH := sectionTopPad + overviewRowH + cardBottomPad
	rankTitleH := 44.0
	rankRows := maxRankingRows
	rankCardH := sectionTopPad + rankTitleH + rankRowH*float64(rankRows) + cardBottomPad
	canvasW := padding*2 + cardW
	canvasH := padding + titleH + sectionGap + overviewCardH + sectionGap + rankCardH + padding

	c := gg.NewContext(int(canvasW), int(canvasH))
	c.SetRGB255(245, 247, 250)
	c.Clear()

	boldFont, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
	if err != nil {
		return nil, err
	}
	regularFont, err := file.GetLazyData(text.FontFile, control.Md5File, true)
	if err != nil {
		return nil, err
	}

	drawCard := func(x, y, w, h float64) {
		c.SetRGBA255(255, 255, 255, 255)
		c.DrawRoundedRectangle(x, y, w, h, cardRadius)
		c.Fill()
		c.SetRGBA255(0, 0, 0, 18)
		c.SetLineWidth(lineW)
		c.DrawRoundedRectangle(x, y, w, h, cardRadius)
		c.Stroke()
	}
	drawSectionTitle := func(title string, x, y float64) error {
		if err := c.ParseFontFace(boldFont, subTitleSize); err != nil {
			return err
		}
		c.SetRGB255(15, 23, 42)
		c.DrawStringAnchored(title, x+20, y+36, 0, 0.5)
		return nil
	}

	// 标题卡片
	drawCard(padding, padding, cardW, titleH)
	if err = c.ParseFontFace(boldFont, titleSize); err != nil {
		return nil, err
	}
	c.SetRGB255(30, 41, 59)
	c.DrawStringAnchored(fmt.Sprintf("个人中心 - %s", amongusID), padding+20, padding+titleH/2, 0, 0.5)

	// 战绩总览卡片：5 项指标横排（与「查询战绩」总览数据一致）
	overviewY := padding + titleH + sectionGap
	drawCard(padding, overviewY, cardW, overviewCardH)
	if err = drawSectionTitle("战绩总览", padding, overviewY); err != nil {
		return nil, err
	}
	overviewItems := [][2]string{
		{fmt.Sprintf("%d", p.Total.TotalMatches), "总场次"},
		{fmt.Sprintf("%.2f%%", p.Total.WinRate), "总胜率"},
		{fmt.Sprintf("%.2f", p.Total.AverageKills), "平均击杀"},
		{fmt.Sprintf("%.2f%%", p.Total.TaskCompletionRate), "任务完成度"},
		{fmt.Sprintf("%.2f%%", p.Total.CompletedAllTasksRate), "全勤任务率"},
	}
	statCellW := cardW / float64(len(overviewItems))
	statCenterY := overviewY + sectionTopPad + overviewRowH/2
	for i, item := range overviewItems {
		centerX := padding + statCellW*(float64(i)+0.5)
		if err = c.ParseFontFace(boldFont, statValueSize); err != nil {
			return nil, err
		}
		c.SetRGB255(37, 99, 235)
		c.DrawStringAnchored(item[0], centerX, statCenterY-18, 0.5, 0.5)
		if err = c.ParseFontFace(regularFont, statLabelSize); err != nil {
			return nil, err
		}
		c.SetRGB255(71, 85, 105)
		c.DrawStringAnchored(item[1], centerX, statCenterY+22, 0.5, 0.5)
	}

	// 战绩排行卡片：击杀 / 被击杀 / 死亡率 三列横排
	rankY := overviewY + overviewCardH + sectionGap
	drawCard(padding, rankY, cardW, rankCardH)
	if err = drawSectionTitle("战绩排行", padding, rankY); err != nil {
		return nil, err
	}

	rankColumns := []struct {
		title       string
		entries     []rankingEntry
		isDeathRate bool
	}{
		{"击杀排行", r.TopVictims, false},
		{"被击杀排行", r.TopKillers, false},
		{"死亡率排行", r.DeathRoles, true},
	}

	for ci, col := range rankColumns {
		colX := padding + 20 + (rankColW+rankColGap)*float64(ci)
		colY := rankY + sectionTopPad

		if err = c.ParseFontFace(boldFont, headerSize); err != nil {
			return nil, err
		}
		c.SetRGB255(51, 65, 85)
		c.DrawStringAnchored(col.title, colX, colY+rankTitleH/2, 0, 0.5)

		entries := col.entries
		if len(entries) > maxRankingRows {
			entries = entries[:maxRankingRows]
		}
		if len(entries) == 0 {
			if err = c.ParseFontFace(regularFont, bodySize); err != nil {
				return nil, err
			}
			c.SetRGB255(148, 163, 184)
			c.DrawStringAnchored("暂无数据", colX, colY+rankTitleH+rankRowH/2, 0, 0.5)
			continue
		}
		for i, e := range entries {
			y := colY + rankTitleH + rankRowH*float64(i)
			if i%2 == 0 {
				c.SetRGBA255(148, 163, 184, 10)
				c.DrawRectangle(colX-8, y, rankColW+8, rankRowH)
				c.Fill()
			}

			// 左侧：名次 + 名称
			if err = c.ParseFontFace(regularFont, bodySize); err != nil {
				return nil, err
			}
			c.SetRGB255(100, 116, 139)
			c.DrawStringAnchored(fmt.Sprintf("%d.", i+1), colX, y+rankRowH/2, 0, 0.5)
			c.SetRGB255(15, 23, 42)
			nameX := colX + 40
			nameMaxW := rankColW - 40 - 110 // 右侧数值区预留 110
			c.DrawStringWrapped(e.Name, nameX, y+rankRowH/2, 0, 0.5, nameMaxW, 1.1, gg.AlignLeft)

			// 右侧：数值
			var value string
			if col.isDeathRate {
				value = fmt.Sprintf("%.2f%%", e.DeathRate)
			} else {
				value = fmt.Sprintf("%d 次", e.Count)
			}
			if err = c.ParseFontFace(boldFont, bodySize); err != nil {
				return nil, err
			}
			c.SetRGB255(37, 99, 235)
			c.DrawStringAnchored(value, colX+rankColW, y+rankRowH/2, 1, 0.5)

			// 死亡率排行的数值下方补充 死亡/总场次
			if col.isDeathRate {
				if err = c.ParseFontFace(regularFont, smallSize); err != nil {
					return nil, err
				}
				c.SetRGB255(148, 163, 184)
				c.DrawStringAnchored(fmt.Sprintf("%d/%d", e.Deaths, e.Total), colX+rankColW, y+rankRowH/2+16, 1, 0.5)
			}
		}
	}

	return imgfactory.ToBytes(c.Image())
}
