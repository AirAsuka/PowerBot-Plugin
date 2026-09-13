// Package dict 提供 amongus 游戏数据
package dict

import (
	_ "embed"
	"strings"

	"github.com/tidwall/gjson"
)

//go:embed role_info.json
var roleInfoJSON []byte

// ExtraRoleDesc 收录不在 Among Us 原始职业数据中的小知识。
// 其中天使和白板来自“谁是卧底”插件，不应加入 Among Us 的职业或阵营映射。
var extraRoleDesc = map[string]string{
	"灵光师": "简介：你看到其他人都是小黑人，使用技能探查他们的阵营\n\n" +
		"详细介绍：灵光师可以感知其他玩家的存在，在游戏的最开始，所有人物在你的眼里都是白色的，而使用技能可以让你得知他的阵营。绿色代表船员，灰色代表中立，红色代表内鬼。然而作为代价，你无法得知这个人究竟是谁。\n\n" +
		"开场白：人与人的悲欢并不相通，我只觉得他们吵闹。",
	"天使": "简介：可以看到双方词语的好人\n\n" +
		"详细介绍：天使隶属于好人阵营，可以同时知道好坏双方的词语。但是天使不知道哪一个是好人词哪一个是坏人词，你需要带领好人阵营推理出坏人和白板",
	"白板": "简介：不知道词语，猜测获得胜利\n\n" +
		"详细介绍：白板隶属于中立阵营，开局不知道任何一方的词语。你需要在白天伪造发言并且推测出好人和坏人的词语。在夜晚通过猜测双方词语获得胜利，猜错则继续游戏，本晚不能继续猜测",
}

// GetRoleDesc 根据中文角色名查找对应的角色描述（中文）
// 依次拼接 ShortDesc、FullDesc、IntroDesc 三段内容。
// 如果一个中文名对应多个英文角色，则返回所有匹配的描述。
// 没有找到返回空字符串。
func GetRoleDesc(chineseName string) string {
	if desc, ok := extraRoleDesc[chineseName]; ok {
		return desc
	}

	enNames, ok := RoleTextReverse[chineseName]
	if !ok || len(enNames) == 0 {
		return ""
	}

	parsed := gjson.ParseBytes(roleInfoJSON)

	type descItem struct {
		suffix string
		label  string
	}
	items := [3]descItem{
		{"ShortDesc", "简介："},
		{"FullDesc", "详细介绍："},
		{"IntroDesc", "开场白："},
	}

	var results []string
	for _, en := range enNames {
		var parts []string
		for _, item := range items {
			desc := parsed.Get(en + item.suffix + ".13")
			if desc.Exists() && desc.String() != "" {
				parts = append(parts, item.label+desc.String())
			}
		}
		if len(parts) > 0 {
			results = append(results, strings.Join(parts, "\n\n"))
		}
	}

	if len(results) == 0 {
		return ""
	}
	return strings.Join(results, "\n\n")
}
