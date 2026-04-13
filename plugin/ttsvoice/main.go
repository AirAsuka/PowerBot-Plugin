// Package ttsvoice MiniMax异步语音合成
package ttsvoice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/sirupsen/logrus"

	"github.com/FloatTech/floatbox/binary"
	fcext "github.com/FloatTech/floatbox/ctxext"
	"github.com/FloatTech/floatbox/web"
	sql "github.com/FloatTech/sqlite"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	pkgerrors "github.com/pkg/errors"
)

func init() {
	var sdb = &storage{}

	en := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Extra:            control.ExtraFromString("ttsvoice"),
		Brief:            "MiniMax语音合成",
		Help: "- 设置语音密钥xxx\n" +
			"- 设置语音接口地址\n" +
			"- 设置语音模型[speech-2.8-hd|speech-2.6-hd|speech-2.8-turbo|speech-2.6-turbo|speech-02-hd|speech-02-turbo]\n" +
			"- 设置语音音色xxx\n" +
			"- 设置语音语速1.0\n" +
			"- 设置语音音量10\n" +
			"- 设置语音音调1.0\n" +
			"- 查看语音配置\n" +
			"- 查看音色列表\n" +
			"- 我的音色xxx\n" +
			"- 语音合成[文本]",
		PrivateDataFolder: "ttsvoice",
	})

	getdb := fcext.DoOnceOnSuccess(func(ctx *zero.Ctx) bool {
		sdb.db = sql.New(en.DataFolder() + "ttsvoice.db")
		err := sdb.db.Open(time.Hour)
		if err == nil {
			err = sdb.db.Create("config", &voiceConfig{})
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return false
			}
			err = sdb.db.Create("uservoice", &userVoice{})
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return false
			}
			return true
		}
		ctx.SendChain(message.Text("[ERROR]:", err))
		return false
	})

	// 管理员命令：设置API密钥
	en.OnPrefix("设置语音密钥", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			apiKey := strings.TrimSpace(ctx.State["args"].(string))
			if apiKey == "" {
				ctx.SendChain(message.Text("请提供API密钥"))
				return
			}
			cfg := sdb.getConfig()
			err := sdb.setConfig(apiKey, cfg.APIURL, cfg.ModelName, cfg.VoiceID, cfg.Speed, cfg.Volume, cfg.Pitch)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置API密钥"))
		})

	// 管理员命令：设置API地址
	en.OnPrefix("设置语音接口地址", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			apiURL := strings.TrimSpace(ctx.State["args"].(string))
			if apiURL == "" {
				ctx.SendChain(message.Text("请提供API地址"))
				return
			}
			cfg := sdb.getConfig()
			err := sdb.setConfig(cfg.APIKey, apiURL, cfg.ModelName, cfg.VoiceID, cfg.Speed, cfg.Volume, cfg.Pitch)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置API地址"))
		})

	// 管理员命令：设置模型
	en.OnPrefix("设置语音模型", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			modelName := strings.TrimSpace(ctx.State["args"].(string))
			if modelName == "" {
				ctx.SendChain(message.Text("请提供模型名称"))
				return
			}
			validModels := map[string]bool{
				"speech-2.8-hd":    true,
				"speech-2.6-hd":    true,
				"speech-2.8-turbo": true,
				"speech-2.6-turbo": true,
				"speech-02-hd":     true,
				"speech-02-turbo":  true,
			}
			if !validModels[modelName] {
				ctx.SendChain(message.Text("无效的模型名称"))
				return
			}
			cfg := sdb.getConfig()
			err := sdb.setConfig(cfg.APIKey, cfg.APIURL, modelName, cfg.VoiceID, cfg.Speed, cfg.Volume, cfg.Pitch)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置模型: ", modelName))
		})

	// 管理员命令：设置默认音色
	en.OnPrefix("设置语音音色", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			voiceID := strings.TrimSpace(ctx.State["args"].(string))
			if voiceID == "" {
				ctx.SendChain(message.Text("请提供音色ID"))
				return
			}
			cfg := sdb.getConfig()
			err := sdb.setConfig(cfg.APIKey, cfg.APIURL, cfg.ModelName, voiceID, cfg.Speed, cfg.Volume, cfg.Pitch)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置默认音色: ", voiceID))
		})

	// 管理员命令：设置语速
	en.OnPrefix("设置语音语速", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			speedStr := strings.TrimSpace(ctx.State["args"].(string))
			if speedStr == "" {
				ctx.SendChain(message.Text("请提供语速值(0.5-2.0)"))
				return
			}
			var speed float64
			_, err := fmt.Sscanf(speedStr, "%f", &speed)
			if err != nil || speed < 0.5 || speed > 2.0 {
				ctx.SendChain(message.Text("语速值无效，请提供0.5-2.0之间的数值"))
				return
			}
			cfg := sdb.getConfig()
			err = sdb.setConfig(cfg.APIKey, cfg.APIURL, cfg.ModelName, cfg.VoiceID, speed, cfg.Volume, cfg.Pitch)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置语速: ", speed))
		})

	// 管理员命令：设置音量
	en.OnPrefix("设置语音音量", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			volStr := strings.TrimSpace(ctx.State["args"].(string))
			if volStr == "" {
				ctx.SendChain(message.Text("请提供音量值(1-10)"))
				return
			}
			var vol int
			_, err := fmt.Sscanf(volStr, "%d", &vol)
			if err != nil || vol < 1 || vol > 10 {
				ctx.SendChain(message.Text("音量值无效，请提供1-10之间的整数"))
				return
			}
			cfg := sdb.getConfig()
			err = sdb.setConfig(cfg.APIKey, cfg.APIURL, cfg.ModelName, cfg.VoiceID, cfg.Speed, vol, cfg.Pitch)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置音量: ", vol))
		})

	// 管理员命令：设置音调
	en.OnPrefix("设置语音音调", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			pitchStr := strings.TrimSpace(ctx.State["args"].(string))
			if pitchStr == "" {
				ctx.SendChain(message.Text("请提供音调值(0.5-2.0)"))
				return
			}
			var pitch float64
			_, err := fmt.Sscanf(pitchStr, "%f", &pitch)
			if err != nil || pitch < 0.5 || pitch > 2.0 {
				ctx.SendChain(message.Text("音调值无效，请提供0.5-2.0之间的数值"))
				return
			}
			cfg := sdb.getConfig()
			err = sdb.setConfig(cfg.APIKey, cfg.APIURL, cfg.ModelName, cfg.VoiceID, cfg.Speed, cfg.Volume, pitch)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置音调: ", pitch))
		})

	// 查看配置（管理员）
	en.OnFullMatch("查看语音配置", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text(sdb.PrintConfig()))
		})

	// 查看音色列表
	en.OnFullMatch("查看音色列表", getdb).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			data, err := RenderVoiceListToBase64()
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Image("base64://" + binary.BytesToString(data)))
		})

	// 用户命令：设置自己的音色
	en.OnPrefix("我的音色", getdb).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			input := strings.TrimSpace(ctx.State["args"].(string))
			if input == "" {
				currentVoice := sdb.getUserVoice(ctx.Event.UserID)
				if currentVoice == "" {
					cfg := sdb.getConfig()
					ctx.SendChain(message.Text("当前使用默认音色: ", cfg.VoiceID))
				} else {
					ctx.SendChain(message.Text("当前音色: ", currentVoice))
				}
				return
			}

			// 解析输入：支持序号、名字、ID
			voiceID, ok := ParseVoiceInput(input)
			if !ok {
				ctx.SendChain(message.Text("未找到匹配的音色，请输入序号(1-58)、名字或ID"))
				return
			}

			err := sdb.setUserVoice(ctx.Event.UserID, voiceID)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("成功设置音色: ", voiceID))
		})

	// 语音合成命令
	en.OnPrefix("语音合成", getdb).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			text := strings.TrimSpace(ctx.State["args"].(string))
			if text == "" {
				ctx.SendChain(message.Text("请提供要合成的文本"))
				return
			}

			cfg := sdb.getConfig()
			if cfg.APIKey == "" || cfg.APIURL == "" || cfg.ModelName == "" {
				ctx.SendChain(message.Text("请先配置API密钥、接口地址和模型"))
				return
			}

			// 获取用户音色，优先使用用户设置的，否则使用默认音色
			voiceID := sdb.getUserVoice(ctx.Event.UserID)
			if voiceID == "" {
				voiceID = cfg.VoiceID
			}
			if voiceID == "" {
				voiceID = "male-qn-qingse" // 备用默认音色
			}

			ctx.SendChain(message.Text("正在合成语音..."))

			// 1. 创建语音合成任务
			taskID, err := sdb.createTask(cfg.APIKey, cfg.APIURL, cfg.ModelName, text, voiceID, cfg.Speed, cfg.Volume, cfg.Pitch)
			logrus.Infoln("[ttsvoice] createTask返回: taskID=", taskID, "err=", err)
			if err != nil {
				ctx.SendChain(message.Text("创建任务失败: ", err))
				return
			}
			if taskID == "" {
				ctx.SendChain(message.Text("创建任务返回为空"))
				return
			}

			// 2. 轮询查询任务状态
			logrus.Infoln("[ttsvoice] taskID:", taskID)
			var fileID string
			for i := 0; i < 30; i++ { // 最多等待30秒
				time.Sleep(time.Second)
				fileID, err = sdb.queryTask(cfg.APIKey, cfg.APIURL, taskID)
				logrus.Infoln("[ttsvoice] 第", i+1, "次查询, fileID:", fileID, "err:", err)
				if err != nil {
					logrus.Warnln("[ttsvoice] 查询任务失败: ", err)
					continue
				}
				if fileID != "" {
					break
				}
			}

			if fileID == "" {
				ctx.SendChain(message.Text("语音合成超时，请稍后重试"))
				return
			}

			// 3. 下载音频
			audioData, err := sdb.downloadAudio(cfg.APIKey, cfg.APIURL, fileID)
			if err != nil {
				ctx.SendChain(message.Text("下载音频失败: ", err))
				return
			}

			// 4. 发送音频
			ctx.SendChain(message.Record(audioData))
		})
}

// createTask 创建语音合成任务
func (sdb *storage) createTask(apiKey, apiURL, model, text, voiceID string, speed float64, vol int, pitch float64) (string, error) {
	reqData := map[string]interface{}{
		"model": model,
		"text":  text,
		"voice_setting": map[string]interface{}{
			"voice_id": voiceID,
			"speed":    speed,
			"vol":      vol,
			"pitch":    pitch,
		},
		"audio_setting": map[string]interface{}{
			"audio_sample_rate": 32000,
			"bitrate":           128000,
			"format":            "mp3",
			"channel":           2,
		},
	}

	reqBytes, _ := json.Marshal(reqData)
	url := apiURL + "/t2a_async_v2"
	resp, err := web.RequestDataWithHeaders(
		web.NewDefaultClient(),
		url,
		"POST",
		func(req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("Content-Type", "application/json")
			return nil
		},
		bytes.NewReader(reqBytes),
	)
	if err != nil {
		return "", err
	}

	logrus.Debugln("[ttsvoice] 创建任务返回:", string(resp))

	respStr := string(resp)
	logrus.Infoln("[ttsvoice] 原始响应:", respStr)

	// 检查API错误
	statusCode := gjson.Get(respStr, "base_resp.status_code").Int()
	statusMsg := gjson.Get(respStr, "base_resp.status_msg").String()
	if statusCode != 0 && statusMsg != "" {
		return "", pkgerrors.New(fmt.Sprintf("API错误 %d: %s", statusCode, statusMsg))
	}

	// 解析task_id (可能是整数或字符串)
	taskID := gjson.Get(respStr, "task_id").String()
	if taskID == "" || taskID == "0" {
		// 尝试其他可能的字段
		taskID = gjson.Get(respStr, "id").String()
		if taskID == "" || taskID == "0" {
			taskID = gjson.Get(respStr, "data.task_id").String()
		}
	}
	logrus.Infoln("[ttsvoice] 解析到的taskID:", taskID)
	if taskID == "" || taskID == "0" {
		return "", pkgerrors.New(fmt.Sprintf("未获取到有效的task_id: %s", respStr))
	}

	return taskID, nil
}

// queryTask 查询任务状态
func (sdb *storage) queryTask(apiKey, apiURL, taskID string) (string, error) {
	url := apiURL + "/query/t2a_async_query_v2?task_id=" + taskID
	resp, err := web.RequestDataWithHeaders(
		web.NewDefaultClient(),
		url,
		"GET",
		func(req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			return nil
		},
		nil,
	)
	if err != nil {
		return "", err
	}

	respStr := string(resp)
	logrus.Infoln("[ttsvoice] 查询任务返回:", respStr)

	status := gjson.Get(respStr, "status").String()
	if status == "Success" || status == "success" {
		// file_id可能在顶层或data里
		fileID := gjson.Get(respStr, "data.file_id").String()
		if fileID == "" || fileID == "0" {
			fileID = gjson.Get(respStr, "file_id").String()
		}
		logrus.Infoln("[ttsvoice] 任务完成, fileID:", fileID)
		return fileID, nil
	}

	// 还在处理中
	logrus.Infoln("[ttsvoice] 任务状态:", status)
	return "", nil
}

// downloadAudio 下载音频文件
func (sdb *storage) downloadAudio(apiKey, apiURL, fileID string) (string, error) {
	url := apiURL + "/files/retrieve_content?file_id=" + fileID
	resp, err := web.RequestDataWithHeaders(
		web.NewDefaultClient(),
		url,
		"GET",
		func(req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			return nil
		},
		nil,
	)
	if err != nil {
		return "", err
	}

	// 保存到临时文件
	tmpDir := os.TempDir()
	fileIDShorthand := fileID
	if len(fileIDShorthand) > 8 {
		fileIDShorthand = fileIDShorthand[:8]
	}
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("tts_%s.mp3", fileIDShorthand))
	err = os.WriteFile(tmpFile, resp, 0644)
	if err != nil {
		return "", err
	}

	return tmpFile, nil
}
