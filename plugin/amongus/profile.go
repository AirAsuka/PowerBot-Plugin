package amongus

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/web"
	"github.com/FloatTech/gg"
	"github.com/FloatTech/imgfactory"
	amongusdict "github.com/FloatTech/ZeroBot-Plugin/plugin/amongus/dict"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/img/text"
	"github.com/tidwall/gjson"
)

// 个人统计数据结构，字段与 TOUE-Web /api/profile/<player_code> 响应保持一致

type playerNameStat struct {
	Name  string
	Count int64
}

type profileTotalStats struct {
	TotalMatches          int64
	WinRate               float64
	AverageKills          float64
	TaskCompletionRate    float64
	CompletedAllTasksRate float64
	AverageKillsByCamp    map[string]float64
}

type campStat struct {
	Camp         amongusdict.CampType
	TotalMatches int64
	WinMatches   int64
	WinRate      float64
	AverageKills float64
}

type roleStat struct {
	RoleName           string
	Config             amongusdict.RoleConfig
	TotalMatches       int64
	WinMatches         int64
	WinRate            float64
	AverageKills       float64
	TaskCompletionRate float64
}

type profileData struct {
	Names []playerNameStat
	Total profileTotalStats
	Camps []campStat // 顺序固定为 船员 / 伪装者 / 中立
	Roles []roleStat
}

// queryPlayerProfile 请求 /api/profile/<player_code> 并解析个人统计
func queryPlayerProfile(amongusID string) (*profileData, error) {
	fullURL := profileAPI + url.PathEscape(amongusID)
	data, err := web.GetData(fullURL)
	if err != nil {
		return nil, err
	}
	result := gjson.ParseBytes(data)
	if !result.Get("success").Bool() {
		return nil, errors.New(errorMessageFromResult(result, "查询战绩失败"))
	}

	profile := &profileData{}

	for _, n := range result.Get("data.playerNames").Array() {
		profile.Names = append(profile.Names, playerNameStat{
			Name:  n.Get("name").String(),
			Count: n.Get("count").Int(),
		})
	}

	total := result.Get("data.totalStats")
	profile.Total = profileTotalStats{
		TotalMatches:          total.Get("totalMatches").Int(),
		WinRate:               total.Get("winRate").Float(),
		AverageKills:          total.Get("averageKills").Float(),
		TaskCompletionRate:    total.Get("taskCompletionRate").Float(),
		CompletedAllTasksRate: total.Get("completedAllTasksRate").Float(),
		AverageKillsByCamp: map[string]float64{
			"crew":     total.Get("averageKillsByCamp.crew").Float(),
			"impostor": total.Get("averageKillsByCamp.impostor").Float(),
			"neutral":  total.Get("averageKillsByCamp.neutral").Float(),
		},
	}

	// 职业统计：过滤掉没有场次的职业，按总场次降序（与 Web 端 sortedRoleStats 一致）
	for _, r := range result.Get("data.roleStats").Array() {
		totalMatches := r.Get("totalMatches").Int()
		if totalMatches <= 0 {
			continue
		}
		profile.Roles = append(profile.Roles, roleStat{
			RoleName:           r.Get("roleName").String(),
			Config:             amongusdict.GetRoleConfig(r.Get("roleName").String()),
			TotalMatches:       totalMatches,
			WinMatches:         r.Get("winMatches").Int(),
			WinRate:            r.Get("winRate").Float(),
			AverageKills:       r.Get("averageKills").Float(),
			TaskCompletionRate: r.Get("taskCompletionRate").Float(),
		})
	}
	sort.SliceStable(profile.Roles, func(i, j int) bool {
		return profile.Roles[i].TotalMatches > profile.Roles[j].TotalMatches
	})

	// 阵营统计：由职业统计按阵营聚合（与 Web 端 calculateCampStats 一致）
	campTotals := map[amongusdict.CampType]*campStat{
		amongusdict.CampCrewmate: {Camp: amongusdict.CampCrewmate, AverageKills: profile.Total.AverageKillsByCamp["crew"]},
		amongusdict.CampImpostor: {Camp: amongusdict.CampImpostor, AverageKills: profile.Total.AverageKillsByCamp["impostor"]},
		amongusdict.CampNeutral:  {Camp: amongusdict.CampNeutral, AverageKills: profile.Total.AverageKillsByCamp["neutral"]},
	}
	for _, role := range profile.Roles {
		cs, ok := campTotals[role.Config.Camp]
		if !ok {
			continue
		}
		cs.TotalMatches += role.TotalMatches
		cs.WinMatches += role.WinMatches
	}
	profile.Camps = []campStat{
		*campTotals[amongusdict.CampCrewmate],
		*campTotals[amongusdict.CampImpostor],
		*campTotals[amongusdict.CampNeutral],
	}
	for i := range profile.Camps {
		if profile.Camps[i].TotalMatches > 0 {
			profile.Camps[i].WinRate = float64(profile.Camps[i].WinMatches) / float64(profile.Camps[i].TotalMatches) * 100
		}
	}

	return profile, nil
}

// playerNamesText 玩家名称展示文本，多个名称用 " / " 连接（与 Web 端一致）
func (p *profileData) playerNamesText() string {
	if len(p.Names) == 0 {
		return "暂无记录"
	}
	names := make([]string, 0, len(p.Names))
	for _, n := range p.Names {
		names = append(names, n.Name)
	}
	return strings.Join(names, " / ")
}

// formatProfileSummary 个人统计文本摘要（图片渲染失败时的兜底）
func formatProfileSummary(amongusID string, p *profileData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("══ 个人统计 - %s ══\n\n", amongusID))
	sb.WriteString("【玩家名称】\n  " + p.playerNamesText() + "\n\n")

	sb.WriteString("【总览数据】\n")
	sb.WriteString(fmt.Sprintf("  总场次:      %d\n", p.Total.TotalMatches))
	sb.WriteString(fmt.Sprintf("  总胜率:      %.2f%%\n", p.Total.WinRate))
	sb.WriteString(fmt.Sprintf("  平均击杀:    %.2f\n", p.Total.AverageKills))
	sb.WriteString(fmt.Sprintf("  任务完成度:  %.2f%%\n", p.Total.TaskCompletionRate))
	sb.WriteString(fmt.Sprintf("  全勤任务率:  %.2f%%\n\n", p.Total.CompletedAllTasksRate))

	sb.WriteString("【阵营统计】\n")
	for _, cs := range p.Camps {
		sb.WriteString(fmt.Sprintf("  %s: 总场次 %d | 胜率 %.2f%% | 平均击杀 %.2f\n",
			amongusdict.GetCampText(cs.Camp), cs.TotalMatches, cs.WinRate, cs.AverageKills))
	}

	sb.WriteString("\n【职业统计】\n")
	sb.WriteString("职业 | 总场次 | 胜场数 | 胜率 | 平均击杀 | 任务完成率\n")
	for _, r := range p.Roles {
		kills := "-"
		if r.Config.HasKill {
			kills = fmt.Sprintf("%.2f", r.AverageKills)
		}
		tasks := "-"
		if r.Config.HasTask {
			tasks = fmt.Sprintf("%.2f%%", r.TaskCompletionRate)
		}
		sb.WriteString(fmt.Sprintf("%s | %d | %d | %.2f%% | %s | %s\n",
			mapOrSelf(roleText, r.RoleName), r.TotalMatches, r.WinMatches, r.WinRate, kills, tasks))
	}
	return strings.TrimSpace(sb.String())
}

// renderProfileImage 渲染个人统计图片：玩家名称 / 总览数据 / 阵营统计 / 职业统计
func renderProfileImage(amongusID string, p *profileData) ([]byte, error) {
	const (
		padding       = 40.0
		titleSize     = 44.0
		subTitleSize  = 32.0
		headerSize    = 24.0
		bodySize      = 22.0
		smallSize     = 18.0
		rowH          = 62.0
		statValueSize = 34.0
		statLabelSize = 20.0
		cardRadius    = 18.0
		lineW         = 2.0
		tableHeaderH  = 56.0
		sectionTopPad = 68.0 // 卡片标题区到内容的距离
		cardBottomPad = 28.0 // 卡片底部留白，防止文字踩边框
	)

	// 职业统计列宽
	colW := []float64{
		300, // 职业（阵营 + 中文名）
		110, // 总场次
		110, // 胜场数
		120, // 胜率
		130, // 平均击杀
		140, // 任务完成率
	}
	tableW := 0.0
	for _, w := range colW {
		tableW += w
	}

	// 各卡片高度
	titleH := 70.0
	nameRowH := 56.0
	nameCardH := sectionTopPad + nameRowH + cardBottomPad
	overviewRowH := 120.0
	overviewCardH := sectionTopPad + overviewRowH + cardBottomPad
	campCardInnerH := 170.0
	campCardH := sectionTopPad + campCardInnerH + cardBottomPad
	roleCardH := sectionTopPad + tableHeaderH + rowH*float64(len(p.Roles)) + cardBottomPad
	sectionGap := 18.0
	canvasW := padding*2 + tableW
	canvasH := padding + titleH + sectionGap + nameCardH + sectionGap + overviewCardH + sectionGap + campCardH + sectionGap + roleCardH + padding

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

	cardW := canvasW - padding*2
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
	c.DrawStringAnchored(fmt.Sprintf("个人统计 - %s", amongusID), padding+20, padding+titleH/2, 0, 0.5)

	// 玩家名称卡片
	nameY := padding + titleH + sectionGap
	drawCard(padding, nameY, cardW, nameCardH)
	if err = drawSectionTitle("玩家名称", padding, nameY); err != nil {
		return nil, err
	}
	if err = c.ParseFontFace(regularFont, bodySize); err != nil {
		return nil, err
	}
	c.SetRGB255(15, 23, 42)
	c.DrawStringWrapped(p.playerNamesText(), padding+20, nameY+sectionTopPad+nameRowH/2, 0, 0.5, cardW-40, 1.3, gg.AlignLeft)

	// 总览数据卡片：5 项指标横排，大数字 + 标签（与 Web 端 statItems 一致）
	overviewY := nameY + nameCardH + sectionGap
	drawCard(padding, overviewY, cardW, overviewCardH)
	if err = drawSectionTitle("总览数据", padding, overviewY); err != nil {
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

	// 阵营统计卡片：船员 / 伪装者 / 中立 三个子卡片横排
	campY := overviewY + overviewCardH + sectionGap
	drawCard(padding, campY, cardW, campCardH)
	if err = drawSectionTitle("阵营统计", padding, campY); err != nil {
		return nil, err
	}
	campColors := map[amongusdict.CampType][3]int{
		amongusdict.CampCrewmate: {37, 99, 235},
		amongusdict.CampImpostor: {220, 38, 38},
		amongusdict.CampNeutral:  {100, 116, 139},
	}
	campGap := 16.0
	campSubW := (cardW - 40 - campGap*2) / 3
	campSubY := campY + sectionTopPad
	for i, cs := range p.Camps {
		subX := padding + 20 + (campSubW+campGap)*float64(i)
		color := campColors[cs.Camp]
		c.SetRGBA255(color[0], color[1], color[2], 12)
		c.DrawRoundedRectangle(subX, campSubY, campSubW, campCardInnerH, 12)
		c.Fill()
		c.SetRGBA255(color[0], color[1], color[2], 90)
		c.SetLineWidth(lineW)
		c.DrawRoundedRectangle(subX, campSubY, campSubW, campCardInnerH, 12)
		c.Stroke()

		if err = c.ParseFontFace(boldFont, headerSize); err != nil {
			return nil, err
		}
		c.SetRGB255(color[0], color[1], color[2])
		c.DrawStringAnchored(amongusdict.GetCampText(cs.Camp), subX+campSubW/2, campSubY+30, 0.5, 0.5)

		campItems := [][2]string{
			{fmt.Sprintf("%d", cs.TotalMatches), "总场次"},
			{fmt.Sprintf("%.2f%%", cs.WinRate), "胜率"},
			{fmt.Sprintf("%.2f", cs.AverageKills), "平均击杀"},
		}
		campCellW := campSubW / 3
		for j, item := range campItems {
			centerX := subX + campCellW*(float64(j)+0.5)
			if err = c.ParseFontFace(boldFont, bodySize); err != nil {
				return nil, err
			}
			c.SetRGB255(color[0], color[1], color[2])
			c.DrawStringAnchored(item[0], centerX, campSubY+90, 0.5, 0.5)
			if err = c.ParseFontFace(regularFont, smallSize); err != nil {
				return nil, err
			}
			c.SetRGB255(71, 85, 105)
			c.DrawStringAnchored(item[1], centerX, campSubY+126, 0.5, 0.5)
		}
	}

	// 职业统计卡片
	roleY := campY + campCardH + sectionGap
	drawCard(padding, roleY, cardW, roleCardH)
	if err = drawSectionTitle("职业统计", padding, roleY); err != nil {
		return nil, err
	}

	headerX := padding + 20
	headerY := roleY + sectionTopPad
	roleTableW := cardW - 40
	c.SetRGBA255(15, 23, 42, 8)
	c.DrawRoundedRectangle(headerX, headerY, roleTableW, tableHeaderH, 12)
	c.Fill()

	headers := []string{"职业", "总场次", "胜场数", "胜率", "平均击杀", "任务完成率"}
	if err = c.ParseFontFace(boldFont, headerSize); err != nil {
		return nil, err
	}
	c.SetRGB255(51, 65, 85)
	x := headerX
	for i, h := range headers {
		if i == 0 {
			c.DrawStringAnchored(h, x+10, headerY+tableHeaderH/2, 0, 0.5)
		} else {
			c.DrawStringAnchored(h, x+colW[i]/2, headerY+tableHeaderH/2, 0.5, 0.5)
		}
		x += colW[i]
	}

	rowStartY := headerY + tableHeaderH
	for i, role := range p.Roles {
		y := rowStartY + rowH*float64(i)
		if i%2 == 0 {
			c.SetRGBA255(148, 163, 184, 10)
			c.DrawRectangle(headerX, y, roleTableW, rowH)
			c.Fill()
		}
		c.SetRGBA255(148, 163, 184, 35)
		c.SetLineWidth(1)
		c.DrawLine(headerX, y+rowH, headerX+roleTableW, y+rowH)
		c.Stroke()

		kills := "-"
		if role.Config.HasKill {
			kills = fmt.Sprintf("%.2f", role.AverageKills)
		}
		tasks := "-"
		if role.Config.HasTask {
			tasks = fmt.Sprintf("%.2f%%", role.TaskCompletionRate)
		}
		cells := []string{
			"", // 职业列单独绘制两行文本
			fmt.Sprintf("%d", role.TotalMatches),
			fmt.Sprintf("%d", role.WinMatches),
			fmt.Sprintf("%.2f%%", role.WinRate),
			kills,
			tasks,
		}

		// 职业列：上行职业中文名，下行阵营名（与 Web 端 role-info 一致）
		if err = c.ParseFontFace(boldFont, bodySize); err != nil {
			return nil, err
		}
		c.SetRGB255(15, 23, 42)
		c.DrawStringAnchored(mapOrSelf(roleText, role.RoleName), headerX+10, y+rowH/2-12, 0, 0.5)
		if err = c.ParseFontFace(regularFont, smallSize); err != nil {
			return nil, err
		}
		c.SetRGB255(100, 116, 139)
		c.DrawStringAnchored(amongusdict.GetCampShortText(role.Config.Camp), headerX+10, y+rowH/2+14, 0, 0.5)

		if err = c.ParseFontFace(regularFont, bodySize); err != nil {
			return nil, err
		}
		x = headerX
		for ci, cell := range cells {
			if ci == 0 {
				x += colW[ci]
				continue
			}
			c.SetRGB255(15, 23, 42)
			c.DrawStringAnchored(cell, x+colW[ci]/2, y+rowH/2, 0.5, 0.5)
			x += colW[ci]
		}
	}

	return imgfactory.ToBytes(c.Image())
}
