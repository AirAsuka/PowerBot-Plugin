package dict

import (
	_ "embed"
	"strings"

	"github.com/tidwall/gjson"
)

//go:embed role_info.json
var roleInfoJSON []byte

// GetRoleFullDesc 根据中文角色名查找对应的角色技能描述（中文）
// 如果一个中文名对应多个英文角色，则返回所有匹配的描述，以分隔符连接。
// 没有找到返回空字符串。
func GetRoleFullDesc(chineseName string) string {
	enNames, ok := RoleTextReverse[chineseName]
	if !ok || len(enNames) == 0 {
		return ""
	}

	parsed := gjson.ParseBytes(roleInfoJSON)

	var results []string
	for _, en := range enNames {
		desc := parsed.Get(en + "FullDesc.13")
		if desc.Exists() && desc.String() != "" {
			results = append(results, desc.String())
		}
	}

	if len(results) == 0 {
		return ""
	}
	return strings.Join(results, "\n\n")
}
