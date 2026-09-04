package undercover

import (
	"errors"
	"strconv"
)

const votePattern = `^卧底投票\s*(?:\[CQ:at,(?:[^\]]*,)?qq=(\d+)(?:,[^\]]*)?\]|(\d+))\s*$`

const nightActionPattern = `^卧底刀人\s+(?:(\d+)\s+)?(不刀|\d+)$`

func voteTarget(matches []string) (int64, error) {
	if len(matches) < 3 {
		return 0, errors.New("无法识别投票目标")
	}
	target := matches[1]
	if target == "" {
		target = matches[2]
	}
	id, err := strconv.ParseInt(target, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("无法识别投票目标")
	}
	return id, nil
}
