package chat

import (
	_ "embed"
	"math/rand"
	"strings"

	"github.com/tidwall/gjson"
)

//go:embed chatdata/voice_data.json
var voiceDataJSON []byte

// pokeVoices 存储所有"戳一下"和"信赖触摸"的文本
var pokeVoices []string

func init() {
	results := gjson.ParseBytes(voiceDataJSON).Array()
	for _, item := range results {
		if poke := item.Get("戳一下").String(); poke != "" {
			pokeVoices = append(pokeVoices, poke)
		}
		if trust := item.Get("信赖触摸").String(); trust != "" {
			pokeVoices = append(pokeVoices, trust)
		}
	}
}

// randVoice 随机获取一条语音文本，并将"博士"替换为指定名字
func randVoice(name string) string {
	text := pokeVoices[rand.Intn(len(pokeVoices))]
	return strings.ReplaceAll(text, "博士", name)
}
