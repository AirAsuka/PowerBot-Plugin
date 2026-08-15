// Package music 整合多平台音乐点播能力
package music

import (
	"fmt"
	"strconv"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/kuwo"
	"github.com/guohuiyuan/music-lib/migu"
	"github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qq"
	"github.com/pkg/errors"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

// songInfo 汇总一首待发送歌曲的全部信息
type songInfo struct {
	cardType string // OneBot 官方音乐卡片类型（163/qq），空表示该平台无官方卡片
	id       string
	name     string
	artist   string
	cover    string
	pageURL  string
	playURL  string
}

var platformMap = map[string]func(string) (*songInfo, error){
	"咪咕": getMiguMusic,
	"酷我": getKuwoMusic,
	"酷狗": getKugouMusic,
	"网易": getNeteaseMusic,
	"qq": getQQMusic,
	"":   getNeteaseMusic, // 默认点歌指向网易（可发官方卡片）
}

func init() {
	control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "点歌",
		Help: "- 点歌[xxx] (默认网易)\n" +
			"- 网易点歌[xxx]\n" +
			"- 酷我点歌[xxx]\n" +
			"- 酷狗点歌[xxx]\n" +
			"- 咪咕点歌[xxx]\n" +
			"- qq点歌[xxx]\n",
	}).OnRegex(`^(.{0,2})点歌\s?(.{1,25})$`).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			matches := ctx.State["regex_matched"].([]string)
			platformPrefix := matches[1]
			keyword := matches[2]

			processFunc, ok := platformMap[platformPrefix]
			if !ok {
				ctx.SendChain(message.Text("不支持的点播平台：", platformPrefix))
				return
			}

			song, err := processFunc(keyword)
			if err != nil && platformPrefix == "" {
				// 默认点歌失败，尝试QQ点歌
				song, err = getQQMusic(keyword)
				if err != nil {
					// QQ点歌也失败，尝试酷我点歌
					song, err = getKuwoMusic(keyword)
				}
			}
			if err != nil {
				ctx.SendChain(message.Text("点歌失败：", err))
				return
			}
			sendSong(ctx, song)
		})
}

// sendSong 优先发送官方音乐卡片，平台不支持或卡片发送失败时降级为文本+封面。
// 自定义音乐卡片（CustomMusic）依赖 ARK 签名，未配置签名的协议端会被 QQ 拒发，
// 因此不再使用。
func sendSong(ctx *zero.Ctx, song *songInfo) {
	if song.cardType != "" {
		if id, err := strconv.ParseInt(song.id, 10, 64); err == nil {
			if msgID := ctx.SendChain(message.Music(song.cardType, id)); msgID.ID() != 0 {
				return
			}
		}
	}
	msg := message.Message{message.Text("🎵 ", song.name, " - ", song.artist, "\n", song.pageURL)}
	if song.playURL != "" {
		msg = append(msg, message.Text("\n直链：", song.playURL))
	}
	if song.cover != "" {
		msg = append(msg, message.Image(song.cover))
	}
	ctx.SendChain(msg...)
}

func getMiguMusic(keyword string) (*songInfo, error) {
	songs, err := migu.Search(keyword)
	if err != nil {
		return nil, errors.Wrap(err, "咪咕音乐搜索失败")
	}
	if len(songs) == 0 {
		return nil, errors.New("咪咕音乐未找到相关歌曲：" + keyword)
	}
	song := songs[0]

	playURL, err := migu.GetDownloadURL(&song)
	if err != nil {
		return nil, errors.Wrap(err, "获取咪咕播放链接失败")
	}
	if playURL == "" {
		return nil, errors.New("获取咪咕播放链接失败：链接为空")
	}

	return &songInfo{
		id:      song.ID,
		name:    song.Name,
		artist:  song.Artist,
		cover:   song.Cover,
		pageURL: fmt.Sprintf("https://music.migu.cn/v3/music/song/%s", song.ID),
		playURL: playURL,
	}, nil
}

func getKuwoMusic(keyword string) (*songInfo, error) {
	songs, err := kuwo.Search(keyword)
	if err != nil {
		return nil, errors.Wrap(err, "酷我音乐搜索失败")
	}
	if len(songs) == 0 {
		return nil, errors.New("酷我音乐未找到相关歌曲：" + keyword)
	}
	song := songs[0]

	playURL, err := kuwo.GetDownloadURL(&song)
	if err != nil {
		return nil, errors.Wrap(err, "获取酷我播放链接失败")
	}
	if playURL == "" {
		return nil, errors.New("获取酷我播放链接失败：链接为空")
	}

	return &songInfo{
		id:      song.ID,
		name:    song.Name,
		artist:  song.Artist,
		cover:   song.Cover,
		pageURL: fmt.Sprintf("https://www.kuwo.cn/play_detail/%s", song.ID),
		playURL: playURL,
	}, nil
}

func getKugouMusic(keyword string) (*songInfo, error) {
	songs, err := kugou.Search(keyword)
	if err != nil {
		return nil, errors.Wrap(err, "酷狗音乐搜索失败")
	}
	if len(songs) == 0 {
		return nil, errors.New("酷狗音乐未找到相关歌曲：" + keyword)
	}
	song := songs[0]

	playURL, err := kugou.GetDownloadURL(&song)
	if err != nil {
		return nil, errors.Wrap(err, "获取酷狗播放链接失败")
	}
	if playURL == "" {
		return nil, errors.New("获取酷狗播放链接失败：链接为空")
	}

	return &songInfo{
		id:      song.ID,
		name:    song.Name,
		artist:  song.Artist,
		cover:   song.Cover,
		pageURL: "https://www.kugou.com/",
		playURL: playURL,
	}, nil
}

func getNeteaseMusic(keyword string) (*songInfo, error) {
	songs, err := netease.Search(keyword)
	if err != nil {
		return nil, errors.Wrap(err, "网易云音乐搜索失败")
	}
	if len(songs) == 0 {
		return nil, errors.New("网易云音乐未找到相关歌曲：" + keyword)
	}
	song := songs[0]

	playURL, err := netease.GetDownloadURL(&song)
	if err != nil {
		return nil, errors.Wrap(err, "获取网易云播放链接失败")
	}
	if playURL == "" {
		return nil, errors.New("获取网易云播放链接失败：链接为空")
	}

	return &songInfo{
		cardType: "163",
		id:       song.ID,
		name:     song.Name,
		artist:   song.Artist,
		cover:    song.Cover,
		pageURL:  fmt.Sprintf("https://music.163.com/#/song?id=%s", song.ID),
		playURL:  playURL,
	}, nil
}

func getQQMusic(keyword string) (*songInfo, error) {
	songs, err := qq.Search(keyword)
	if err != nil {
		return nil, errors.Wrap(err, "QQ音乐搜索失败")
	}
	if len(songs) == 0 {
		return nil, errors.New("QQ音乐未找到相关歌曲：" + keyword)
	}
	song := songs[0]

	playURL, err := qq.GetDownloadURL(&song)
	if err != nil {
		return nil, errors.Wrap(err, "获取QQ音乐播放链接失败")
	}
	if playURL == "" {
		return nil, errors.New("获取QQ音乐播放链接失败：链接为空")
	}

	return &songInfo{
		cardType: "qq",
		id:       song.ID,
		name:     song.Name,
		artist:   song.Artist,
		cover:    song.Cover,
		pageURL:  fmt.Sprintf("https://y.qq.com/n/ryqq/songDetail/%s", song.ID),
		playURL:  playURL,
	}, nil
}
