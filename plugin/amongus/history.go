package amongus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/FloatTech/floatbox/web"
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

			detailText, err := queryGameDetail(gameID)
			if err != nil {
				ctx.SendChain(message.Text("[amongus] 查询游戏详情失败: ", err))
				return
			}
			ctx.SendChain(message.Text(detailText))
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
		sb.WriteString(fmt.Sprintf("======================================================\n")))
	}
	return strings.TrimSpace(sb.String())
}

func queryGameDetail(gameID string) (string, error) {
	encodedGameID := url.PathEscape(gameID)
	fullURL := detailAPI + encodedGameID

	data, err := web.GetData(fullURL)
	if err != nil {
		return "", err
	}

	result := gjson.ParseBytes(data)
	if !result.Get("success").Bool() {
		return "", fmt.Errorf(errorMessageFromResult(result, "查询游戏详情失败"))
	}

	var out bytes.Buffer
	if err = json.Indent(&out, data, "", "  "); err != nil {
		return string(data), nil
	}
	return out.String(), nil
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
