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
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	queryAPI  = "https://toue.mxyx.club/api/query"
	detailAPI = "https://api.toue.mxyx.club/api/detail/"
)

var winConditionText = map[string]string{
	"CanceledEnd":    "房主结束游戏",
	"EveryoneDied":   "无人生还",
	"CrewmateWin":    "船员胜利",
	"ImpostorWin":    "伪装者胜利",
	"JesterWin":      "小丑胜利",
	"DoomsayerWin":   "末日预言家获胜!",
	"ArsonistWin":    "纵火犯胜利",
	"VultureWin":     "秃鹫胜利",
	"LawyerSoloWin":  "律师单独获胜!",
	"PelicanWin":     "鹈鹕胜利！",
	"WerewolfWin":    "月下狼人获胜!",
	"WitnessWin":     "污点证人胜利",
	"JuggernautWin":  "天启获胜!",
	"SwooperWin":     "隐身人获胜!",
	"ExecutionerWin": "处刑者胜利",
	"InfectedWin":    "感染者胜利",
	"LoversTeamWin":  "船员和恋人获胜",
	"LoversSoloWin":  "恋人胜利",
	"JackalWin":      "豺狼阵营胜利",
	"PavlovsWin":     "巴甫洛夫阵营胜利",
	"AkujoSoloWin":   "魅魔阵营胜利",
	"AkujoTeamWin":   "魅魔跟随船员阵营胜利",
	"MiniLose":       "无人胜利，小孩被投出",
	"LawyerStolenWin": "律师代替客户胜利",
	"LawyerBonusWin":  "律师和客户胜利",
	"PartTimerWin":    "打工仔跟随胜利",
	"BandLeaderWin":   "乐队跟随胜利",
}

var roleText = map[string]string{
	// 阵营
	"Crewmate": "船员",
	"Impostor": "伪装者",
	"Neutral":  "中立",
	// 角色与修饰
	"Assassin":             "刺客",
	"WolfLord":             "狼之主",
	"Morphling":            "化形者",
	"Bomber":               "炸弹狂",
	"Mimic":                "模仿者",
	"Camouflager":          "隐蔽者",
	"Poucher":              "入殓师",
	"Butcher":              "肢解者",
	"Miner":                "管道工",
	"Eraser":               "抹除者",
	"Vampire":              "吸血鬼",
	"Cleaner":              "清理者",
	"Undertaker":           "送葬者",
	"Escapist":             "逃逸者",
	"Warlock":              "术士",
	"Trickster":            "骗术师",
	"BountyHunter":         "赏金猎人",
	"Terrorist":            "恐怖分子",
	"Blackmailer":          "勒索者",
	"Witch":                "女巫",
	"Ninja":                "忍者",
	"Professional":         "职业杀手",
	"Yoyo":                 "悠悠球",
	"EvilTrapper":          "邪恶的设陷师",
	"Gambler":              "赌徒",
	"Grenadier":            "炸弹狂",
	"Gunsmith":             "快枪手",
	"Berserker":            "狂战士",
	"Survivor":             "幸存者",
	"Amnisiac":             "失忆者",
	"Jester":               "小丑",
	"Vulture":              "秃鹫",
	"Lawyer":               "律师",
	"Executioner":          "处刑者",
	"Pursuer":              "起诉人",
	"Witness":              "污点证人",
	"BandLeader":           "乐队主唱",
	"PartTimer":            "打工仔",
	"Jackal":               "豺狼",
	"Sidekick":             "跟班",
	"Pavlovsowner":         "巴甫洛夫",
	"Pavlovsdogs":          "巴甫洛夫的狗",
	"Swooper":              "隐身人",
	"Arsonist":             "纵火犯",
	"Werewolf":             "月下狼人",
	"SchrodingersCat":      "薛定谔的猫",
	"Thief":                "身份窃贼",
	"Juggernaut":           "天启",
	"Pelican":              "鹈鹕",
	"Doomsayer":            "末日预言家",
	"Akujo":                "魅魔",
	"Vigilante":            "侠客",
	"Mayor":                "市长",
	"Prosecutor":           "检察官",
	"Portalmaker":          "星门缔造者",
	"Marionette":           "木偶师",
	"Avenger":              "复仇者",
	"Engineer":             "工程师",
	"Sheriff":              "警长",
	"Deputy":               "捕快",
	"BodyGuard":            "保镖",
	"Jumper":               "传送师",
	"Detective":            "侦探",
	"Veteran":              "老兵",
	"Oracle":               "神谕者",
	"Medic":                "法医",
	"Swapper":              "换票师",
	"Seer":                 "灵媒",
	"Hacker":               "黑客",
	"Tracker":              "追踪者",
	"Snitch":               "告密者",
	"Spy":                  "卧底",
	"SecurityGuard":        "保安",
	"Medium":               "通灵师",
	"Trapper":              "设陷师",
	"Prophet":              "预言家",
	"InfoSleuth":           "情报师",
	"Balancer":             "大神官",
	"Redemptor":            "牧师",
	"Infected":             "感染者",
	"Jailor":               "典狱长",
	"Disperser":            "分散者",
	"LastImpostor":         "绝境者",
	"Specoality":           "专业刺客",
	"Vortox":               "迷乱旋涡",
	"Bloody":               "溅血者",
	"AntiTeleport":         "通信兵",
	"TieBreaker":           "破平者",
	"Bait":                 "诱饵",
	"Aftermath":            "余波",
	"Sunglasses":           "太阳镜",
	"Torch":                "火炬",
	"Flash":                "闪电侠",
	"Multitasker":          "多线程",
	"Lover":                "恋人",
	"Giant":                "巨人",
	"Mini":                 "小孩",
	"Vip":                  "VIP",
	"Indomitable":          "不屈者",
	"Cursed":               "反骨",
	"Chameleon":            "变色龙",
	"Tunneler":             "隧道工",
	"Invert":               "酒鬼",
	"Blind":                "胆小鬼",
	"Watcher":              "窥视者",
	"Radar":                "雷达",
	"Clog":                 "阻挠鬼",
	"GhostEngineer":        "灵魂工程师",
	"ButtonBarry":          "执扭人",
	"Specter":              "怨灵",
	"Slueth":               "掘墓人",
	"Tiebreaker":           "破平者",
	"Poltergeist":          "捣蛋鬼",
	"ProfessionalModifier": "职业杀手",
	"PoucherModifier":      "入殓师",
}

type recentGame struct {
	GameID       string
	StartTime    string
	Duration     string
	PlayerCount  int64
	WinCondition string
}

func init() {
	// 最近n场
	engine.OnRegex(`^最近\s*([0-9]{1,2})\s*场$`, getDB).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			nText := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
			n, err := strconv.Atoi(nText)
			if err != nil || n <= 0 || n > 10 {
				ctx.SendChain(message.Text("参数错误：n必须是1到10之间"))
				return
			}

			user, err := database.find(ctx.Event.UserID)
			if err != nil || user.AmongusID == "" {
				ctx.SendChain(message.Text("你还没有录入AmongUs ID，请先使用「录入信息 xxxx」绑定"))
				return
			}

			games, err := queryRecentGames(user.AmongusID, n)
			if err != nil {
				ctx.SendChain(message.Text("[amongus] 查询失败: ", err))
				return
			}
			if len(games) == 0 {
				ctx.SendChain(message.Text("未查询到最近对局记录"))
				return
			}
			ctx.SendChain(message.Text(formatRecentGames(user.AmongusID, games)))
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

				games, err := queryRecentGames(user.AmongusID, 1)
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

func queryRecentGames(amongusID string, n int) ([]recentGame, error) {
	encodedID := url.PathEscape(amongusID)
	fullURL := fmt.Sprintf("%s?playerCode=%s&page=1&pageSize=%d", queryAPI, encodedID, n)

	data, err := web.GetData(fullURL)
	if err != nil {
		return nil, err
	}

	result := gjson.ParseBytes(data)
	if !result.Get("success").Bool() {
		return nil, fmt.Errorf(errorMessageFromResult(result, "查询最近对局失败"))
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
	return games, nil
}

func formatRecentGames(amongusID string, games []recentGame) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("玩家ID：%s\n", amongusID))
	for _, game := range games {
		winText, ok := winConditionText[game.WinCondition]
		if !ok {
			winText = "未知"
		}
		sb.WriteString(fmt.Sprintf("| 游戏ID：%s | 开始时间：%s | 游戏时长：%s | 玩家数量：%d | 胜利信息：%s|\n",
			game.GameID, game.StartTime, game.Duration, game.PlayerCount, winText))
		sb.WriteString("=================================\n")
	}
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
		rowH          = 42.0
		globalRowH    = 54.0
		cardRadius    = 18.0
		lineW         = 2.0
		tableHeaderH  = 52.0
		maxTextWidth  = 240.0
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
	globalBlockH := 40.0 + globalRowH*3 // 3行网格（每行3组信息，更不拥挤）
	tableH := tableHeaderH + rowH*float64(len(players))
	canvasW := padding*2 + tableW
	canvasH := padding + titleH + sectionGap + globalBlockH + sectionGap + (40.0 + tableH) + padding

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
	rows := [][][3]string{
		{
			{"游戏版本  ", global.Get("GameVersion").String()},
			{"房主  ", global.Get("HostPlayer").String()},
			{"房间代码  ", global.Get("RoomCode").String()},
		},
		{
			{"开始时间  ", startTimeStr},
			{"结束时间  ", endTimeStr},
			{"游戏时长  ", duration},
		},
		{
			{"胜利条件  ", winText(global.Get("WinCondition").String())},
			{"玩家数量  ", playerCount},
			{"房主代码  ", global.Get("HostCode").String()},
		},
	}

	gridTop := globalY + 64
	gridLeft := globalX + 20
	gridW := cardW - 40
	cellW := gridW / 3

	drawKV := func(x, y float64, k, v string) error {
		if err := c.ParseFontFace(boldFont, headerSize); err != nil {
			return err
		}
		c.SetRGB255(71, 85, 105)
		c.DrawStringAnchored(k, x, y, 0, 0.5)
		if err := c.ParseFontFace(regularFont, bodySize); err != nil {
			return err
		}
		c.SetRGB255(15, 23, 42)
		c.DrawStringWrapped(v, x+86, y, 0, 0.5, cellW-100, 1.4, gg.AlignLeft)
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
	tableCardH := 40.0 + tableH
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
	headerY := tableY + 60
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
