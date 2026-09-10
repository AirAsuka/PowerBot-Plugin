package werewolf

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
)

// The catalog follows NetEase's classic boards, not limited-time/awakened boards.
// Sources and the bot's chat-specific flow are documented in README.md.
type board struct {
	Name            string
	Players         int
	Roles           map[role]int
	RandomGods      bool
	Slaughter       bool
	FirstNightPeace bool
	WitchSelfSave   bool
	RevealDeaths    bool
}

func setup(name string, n, wolves, villagers int, gods ...role) board {
	b := board{Name: name, Players: n, Roles: map[role]int{}, WitchSelfSave: n < 12}
	if wolves > 0 {
		b.Roles[roleWolf] = wolves
	}
	if villagers > 0 {
		b.Roles[roleVillager] = villagers
	}
	for _, r := range gods {
		b.Roles[r]++
	}
	return b
}

var boards = func() []board {
	end := setup("8人末日捍卫", 8, 3, 2, roleSeer, roleGuard, roleKnight)
	end.Slaughter, end.FirstNightPeace = true, true
	gods := setup("8人诸神黄昏", 8, 0, 0, roleWolfKing, roleWhiteWolfKing, roleEvilKnight, roleSeer, roleWitch, roleHunter, roleGuard, roleIdiot)
	gods.Slaughter, gods.RevealDeaths = true, true
	mask := setup("10人假面之夜", 10, 3, 4)
	mask.RandomGods = true
	hunt := setup("10人狩猎潜狼", 10, 3, 0, roleHunter, roleHunter, roleHunter, roleHunter, roleHunter, roleHunter, roleHunter)
	hunt.Slaughter = true
	white := setup("12人纯白夜影", 12, 3, 4, roleWolfWizard, rolePureSeer, roleWitch, roleHunter, roleGuard)
	white.WitchSelfSave = true
	return []board{
		end, gods,
		setup("9人暗牌场", 9, 3, 3, roleSeer, roleWitch, roleHunter),
		setup("10人速推场", 10, 3, 4, roleSeer, roleWitch, roleHunter),
		setup("10人白狼王骑士", 10, 2, 3, roleWhiteWolfKing, roleSeer, roleWitch, roleKnight, roleIdiot),
		hunt, mask,
		setup("10人奇迹商人", 10, 2, 4, roleWolfKing, roleSeer, roleWitch, roleMerchant),
		setup("10人纯白夜影", 10, 2, 4, roleWolfWizard, rolePureSeer, roleWitch, roleGuard),
		setup("12人标准场", 12, 4, 4, roleSeer, roleWitch, roleHunter, roleIdiot),
		setup("12人预女猎守", 12, 4, 4, roleSeer, roleWitch, roleHunter, roleGuard),
		setup("12人狼王守卫", 12, 3, 4, roleWolfKing, roleSeer, roleWitch, roleHunter, roleGuard),
		setup("12人白狼王守卫", 12, 3, 4, roleWhiteWolfKing, roleSeer, roleWitch, roleHunter, roleGuard),
		setup("12人白狼王骑士", 12, 3, 4, roleWhiteWolfKing, roleSeer, roleWitch, roleKnight, roleGuard),
		setup("12人狼美人骑士", 12, 3, 4, roleWolfBeauty, roleSeer, roleWitch, roleKnight, roleGuard),
		setup("12人恶夜骑士", 12, 3, 4, roleEvilKnight, roleSeer, roleWitch, roleHunter, roleGuard),
		setup("12人狼王摄梦人", 12, 3, 4, roleWolfKing, roleSeer, roleWitch, roleHunter, roleDreamer),
		setup("12人噩梦之影", 12, 3, 4, roleNightmare, roleSeer, roleWitch, roleHunter, roleDreamer),
		setup("12人石像鬼守墓人", 12, 3, 4, roleGargoyle, roleSeer, roleWitch, roleHunter, roleGravekeeper),
		white,
		setup("12人奇迹商人", 12, 3, 4, roleWolfKing, roleSeer, roleWitch, roleGuard, roleMerchant),
	}
}()

func supportedPlayerCount(n int) bool { return n == 8 || n == 9 || n == 10 || n == 12 }

func defaultBoard(n int) string {
	switch n {
	case 8:
		return "8人末日捍卫"
	case 9:
		return "9人暗牌场"
	case 10:
		return "10人速推场"
	case 12:
		return "12人标准场"
	default:
		return ""
	}
}

func findBoard(name string) (board, bool) {
	name = strings.ReplaceAll(strings.TrimSpace(name), " ", "")
	name = strings.ReplaceAll(name, "恶灵骑士", "恶夜骑士")
	if name == "12人预女猎白" {
		name = "12人标准场"
	}
	for _, b := range boards {
		if b.Name == name {
			return b, true
		}
	}
	return board{}, false
}

func (b board) counts() map[role]int {
	c := make(map[role]int, len(b.Roles)+3)
	for r, n := range b.Roles {
		c[r] = n
	}
	if b.RandomGods {
		pool := []role{roleSeer, roleWitch, roleHunter, roleGuard, roleIdiot}
		rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		for _, r := range pool[:3] {
			c[r]++
		}
	}
	return c
}

func countsText(c map[role]int) string {
	var parts []string
	for r := roleVillager; r < roleEnd; r++ {
		if c[r] > 0 {
			parts = append(parts, fmt.Sprintf("%s×%d", r, c[r]))
		}
	}
	return strings.Join(parts, "、")
}

func (b board) description() string {
	text := b.Name + "：" + countsText(b.Roles)
	if b.RandomGods {
		text += "；预言家/女巫/猎人/守卫/愚者随机选3神"
	}
	if b.Slaughter {
		text += "；屠城"
	} else {
		text += "；屠边"
	}
	if b.FirstNightPeace {
		text += "；首夜无刀"
	}
	if b.WitchSelfSave {
		text += "；女巫仅首夜可自救"
	} else {
		text += "；女巫不可自救"
	}
	if b.RevealDeaths {
		text += "；出局翻牌"
	}
	return text
}

func boardListText() string {
	lines := []string{"网易经典板子（仅8、9、10、12人；11人不能开局）："}
	for _, b := range boards {
		lines = append(lines, b.description())
	}
	return strings.Join(lines, "\n") + "\n房主开局前发送：狼人杀板子 完整名称；或：狼人杀板子 自动"
}

func (g *game) selectBoard(actor int64, name string) error {
	if g.Phase != phaseLobby {
		return errGameStarted
	}
	if actor != g.HostID {
		return errNotHost
	}
	if name == "自动" {
		g.BoardName = ""
		g.touch()
		return nil
	}
	b, ok := findBoard(name)
	if !ok {
		return errors.New("未知板子，请发送“狼人杀板子”查看完整名称；不支持11人板子")
	}
	g.BoardName = b.Name
	g.touch()
	return nil
}

func (g *game) boardRules() board {
	if b, ok := findBoard(g.BoardName); ok {
		return b
	}
	// Undealt lobbies and small unit-test fixtures have no selected board.
	return board{WitchSelfSave: true}
}

func (g *game) setupDescription() string {
	name := g.BoardName
	if name == "" {
		name = defaultBoard(len(g.Players))
	}
	if b, ok := findBoard(name); ok {
		return b.description()
	}
	return "当前人数无对应板子，只支持8、9、10、12人"
}
