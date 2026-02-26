package dict

import (
	_ "embed"
	"strings"

	"github.com/tidwall/gjson"
)

//go:embed role_info.json
var roleInfoJSON []byte

// GetRoleDesc 根据中文角色名查找对应的角色描述（中文）
// 依次拼接 ShortDesc、FullDesc、IntroDesc 三段内容。
// 如果一个中文名对应多个英文角色，则返回所有匹配的描述。
// 没有找到返回空字符串。
func GetRoleDesc(chineseName string) string {
	enNames, ok := RoleTextReverse[chineseName]
	if !ok || len(enNames) == 0 {
		return ""
	}

	parsed := gjson.ParseBytes(roleInfoJSON)

	suffixes := [3]string{"ShortDesc", "FullDesc", "IntroDesc"}

	var results []string
	for _, en := range enNames {
		var parts []string
		for _, suffix := range suffixes {
			desc := parsed.Get(en + suffix + ".13")
			if desc.Exists() && desc.String() != "" {
				parts = append(parts, desc.String())
			}
		}
		if len(parts) > 0 {
			results = append(results, strings.Join(parts, "\n"))
		}
	}

	if len(results) == 0 {
		return ""
	}
	return strings.Join(results, "\n\n")
}
