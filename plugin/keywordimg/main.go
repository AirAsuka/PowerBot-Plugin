// Package keywordimg 关键词图片插件
package keywordimg

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/web"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// KeywordImage 关键词图片结构
type KeywordImage struct {
	Keyword  string `json:"keyword"`
	ImageURL string `json:"image_url"` // 本地图片路径
}

// 插件数据
var (
	keywordData = make(map[string]string) // keyword -> localImagePath
	dataFile    string
	imagesDir   string
	RWMutex     sync.RWMutex
	loadOnce    sync.Once
)

func init() {
	engine := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "关键词图片",
		Help: "- 加关键词 xxx [图片]\n" +
			"- 删关键词 xxx\n" +
			"- 查看关键词列表\n" +
			"\n当对话中检测到已添加的关键词时，自动返回对应的图片",
		PrivateDataFolder: "keywordimg",
	})

	dataFile = file.BOTPATH + "/" + engine.DataFolder() + "keywords.json"
	imagesDir = file.BOTPATH + "/" + engine.DataFolder() + "images"

	loadData()
	os.MkdirAll(imagesDir, 0755)

	// 关键词检测
	engine.OnMessage(func(ctx *zero.Ctx) bool {
		text := ctx.Event.Message.ExtractPlainText()
		if text == "" {
			return false
		}
		if strings.HasPrefix(text, "加关键词") || strings.HasPrefix(text, "删关键词") || strings.HasPrefix(text, "查看关键词") {
			return false
		}
		RWMutex.RLock()
		defer RWMutex.RUnlock()
		for keyword := range keywordData {
			if strings.Contains(text, keyword) {
				ctx.State["matched_keyword"] = keyword
				return true
			}
		}
		return false
	}, zero.OnlyGroup).SetBlock(false).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		keyword := ctx.State["matched_keyword"].(string)
		RWMutex.RLock()
		imagePath, ok := keywordData[keyword]
		RWMutex.RUnlock()
		if ok {
			ctx.SendChain(message.Image("file:///" + imagePath))
		}
	})

	// 加关键词
	engine.OnRegex(`^加关键词\s+(.+?)\s*\[CQ`, zero.OnlyGroup, zero.AdminPermission, zero.MustProvidePicture).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			id := ctx.Event.MessageID
			keyword := ctx.State["regex_matched"].([]string)[1]
			if keyword == "" {
				ctx.Send(message.ReplyWithMessage(id, message.Text("请使用: 加关键词 xxx")))
				return
			}

			picURL := ctx.State["image_url"].([]string)[0]
			picData, err := web.GetData(picURL)
			if err != nil {
				ctx.SendChain(message.Text("下载失败: ", err))
				return
			}

			_, format, err := image.DecodeConfig(strings.NewReader(string(picData)))
			if err != nil {
				ctx.SendChain(message.Text("图片格式错误"))
				return
			}

			filename := fmt.Sprintf("%s.%s", keyword, format)
			localPath := filepath.Join(imagesDir, filename)
			os.WriteFile(localPath, picData, 0644)
			fmt.Println("[keywordimg] 保存到:", localPath, "大小:", len(picData))

			RWMutex.Lock()
			if oldPath, ok := keywordData[keyword]; ok {
				os.Remove(oldPath)
			}
			keywordData[keyword] = localPath
			saveData()
			RWMutex.Unlock()

			ctx.Send(message.ReplyWithMessage(id, message.Text("关键词 [", keyword, "] 添加成功")))
		})

	// 删关键词
	engine.OnPrefix("删关键词", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		args := strings.TrimSpace(ctx.State["args"].(string))
		RWMutex.Lock()
		defer RWMutex.Unlock()
		if imagePath, ok := keywordData[args]; ok {
			os.Remove(imagePath)
			delete(keywordData, args)
			saveData()
			ctx.SendChain(message.Text("关键词 [", args, "] 已删除"))
		} else {
			ctx.SendChain(message.Text("关键词 [", args, "] 不存在"))
		}
	})

	// 查看列表
	engine.OnFullMatch("查看关键词列表", zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		RWMutex.RLock()
		defer RWMutex.RUnlock()
		if len(keywordData) == 0 {
			ctx.SendChain(message.Text("当前没有任何关键词"))
			return
		}
		var list strings.Builder
		list.WriteString("当前关键词列表:\n")
		for keyword := range keywordData {
			list.WriteString("- ")
			list.WriteString(keyword)
			list.WriteString("\n")
		}
		ctx.SendChain(message.Text(list.String()))
	})
}

// loadData 从文件加载关键词数据
func loadData() {
	loadOnce.Do(func() {
		dir := filepath.Dir(dataFile)
		os.MkdirAll(dir, 0755)
		data, err := os.ReadFile(dataFile)
		if err != nil {
			return
		}
		var keywords []KeywordImage
		if json.Unmarshal(data, &keywords) != nil {
			return
		}
		for _, kw := range keywords {
			if _, err := os.Stat(kw.ImageURL); err == nil {
				keywordData[kw.Keyword] = kw.ImageURL
			}
		}
	})
}

// saveData 保存关键词数据到文件
func saveData() {
	keywords := make([]KeywordImage, 0, len(keywordData))
	for keyword, imagePath := range keywordData {
		keywords = append(keywords, KeywordImage{Keyword: keyword, ImageURL: imagePath})
	}
	data, _ := json.Marshal(keywords)
	dir := filepath.Dir(dataFile)
	os.MkdirAll(dir, 0755)
	tmpFile := dataFile + ".tmp"
	os.WriteFile(tmpFile, data, 0644)
	os.Rename(tmpFile, dataFile)
}
