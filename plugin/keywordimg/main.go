// Package keywordimg 关键词图片插件
package keywordimg

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FloatTech/floatbox/file"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// KeywordImage 关键词图片结构
type KeywordImage struct {
	Keyword string `json:"keyword"`
	Image   string `json:"image"` // 本地图片路径
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
			"- 加关键词 xxx (回复图片)\n" +
			"- 删关键词 xxx\n" +
			"- 查看关键词列表\n" +
			"\n当对话中检测到已添加的关键词时，自动返回对应的图片",
		PrivateDataFolder: "keywordimg",
	})

	// 初始化数据文件路径
	dataFile = engine.DataFolder() + "keywords.json"
	imagesDir = engine.DataFolder() + "images"

	// 加载数据
	loadData()

	// 确保图片目录存在
	os.MkdirAll(imagesDir, 0755)

	// 关键词检测 - 在群聊中检测消息是否包含关键词
	engine.OnMessage(filter, zero.OnlyGroup).SetBlock(false).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		keyword := ctx.State["matched_keyword"].(string)
		RWMutex.RLock()
		imagePath, ok := keywordData[keyword]
		RWMutex.RUnlock()
		if ok && imagePath != "" {
			// 发送关键词对应的图片
			ctx.SendChain(message.Image("file:///" + imagePath))
		}
	})

	// 加关键词命令 (需要管理员权限)
	engine.OnPrefix("加关键词", zero.OnlyGroup, zero.AdminPermission, zero.MustProvidePicture).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			args := ctx.State["args"].(string)

			// 获取图片URL
			urls := ctx.State["image_url"].([]string)
			if len(urls) == 0 {
				ctx.SendChain(message.Text("没有检测到图片"))
				return
			}
			imageURL := urls[0]

			// 解析关键词
			keyword := strings.TrimSpace(args)
			if keyword == "" {
				ctx.SendChain(message.Text("请指定关键词，格式: 加关键词 xxx"))
				return
			}

			// 生成文件名
			hash := md5hash(imageURL)
			ext := getExt(imageURL)
			filename := fmt.Sprintf("%s_%s%s", keyword, hash, ext)
			localPath := filepath.Join(imagesDir, filename)

			// 下载图片
			ctx.SendChain(message.Text("正在下载图片..."))
			err := file.DownloadTo(imageURL, localPath)
			if err != nil {
				ctx.SendChain(message.Text("下载图片失败: ", err))
				return
			}

			// 保存关键词
			RWMutex.Lock()
			if oldPath, ok := keywordData[keyword]; ok {
				os.Remove(oldPath)
			}
			keywordData[keyword] = localPath
			saveData()
			RWMutex.Unlock()

			ctx.SendChain(message.Text("关键词 [", keyword, "] 添加成功"))
		})

	// 删关键词命令 (需要管理员权限)
	engine.OnPrefix("删关键词", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		args := strings.TrimSpace(ctx.State["args"].(string))
		if args == "" {
			ctx.SendChain(message.Text("请指定要删除的关键词"))
			return
		}

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

	// 查看关键词列表命令
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

// filter 消息过滤器，检测消息中是否包含关键词
func filter(ctx *zero.Ctx) bool {
	// 获取消息纯文本内容
	text := ctx.Event.Message.ExtractPlainText()
	if text == "" {
		return false
	}

	// 跳过以命令前缀开头的消息
	if strings.HasPrefix(text, "加关键词") || strings.HasPrefix(text, "删关键词") || strings.HasPrefix(text, "查看关键词") {
		return false
	}

	RWMutex.RLock()
	defer RWMutex.RUnlock()

	// 检查消息是否包含任何关键词
	for keyword := range keywordData {
		if strings.Contains(text, keyword) {
			ctx.State["matched_keyword"] = keyword
			return true
		}
	}
	return false
}

// getExt 获取图片扩展名
func getExt(url string) string {
	url = strings.ToLower(url)
	idx := strings.LastIndex(url, ".")
	if idx == -1 {
		return ".jpg"
	}
	ext := url[idx:]
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return ext
	}
	return ".jpg"
}

// loadData 从文件加载关键词数据
func loadData() {
	loadOnce.Do(func() {
		// 确保数据目录存在
		dir := filepath.Dir(dataFile)
		os.MkdirAll(dir, 0755)

		data, err := os.ReadFile(dataFile)
		if err != nil {
			return
		}

		var keywords []KeywordImage
		if err := json.Unmarshal(data, &keywords); err != nil {
			return
		}

		for _, kw := range keywords {
			// 验证图片文件是否存在
			if _, err := os.Stat(kw.Image); err == nil {
				keywordData[kw.Keyword] = kw.Image
			}
		}
	})
}

// saveData 保存关键词数据到文件
func saveData() {
	keywords := make([]KeywordImage, 0, len(keywordData))
	RWMutex.RLock()
	for keyword, imagePath := range keywordData {
		keywords = append(keywords, KeywordImage{
			Keyword: keyword,
			Image:   imagePath,
		})
	}
	RWMutex.RUnlock()

	data, err := json.Marshal(keywords)
	if err != nil {
		return
	}

	// 确保目录存在
	dir := filepath.Dir(dataFile)
	os.MkdirAll(dir, 0755)

	// 写入文件
	tmpFile := dataFile + ".tmp"
	os.WriteFile(tmpFile, data, 0644)
	os.Rename(tmpFile, dataFile)
}

// md5hash 计算字符串的MD5哈希
func md5hash(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}
