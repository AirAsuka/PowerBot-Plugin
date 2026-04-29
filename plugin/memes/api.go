// Package memes 表情包制作插件
package memes

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	pkgerrors "github.com/pkg/errors"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

func fetchMemeInfos() ([]MemeInfo, error) {
	resp, err := httpClient.Get(baseURL + "/meme/infos")
	if err != nil {
		return nil, pkgerrors.Wrap(err, "获取表情信息失败")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, pkgerrors.New(fmt.Sprintf("获取表情信息失败(HTTP %d): %s", resp.StatusCode, string(body)))
	}

	var infos []MemeInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		return nil, pkgerrors.Wrap(err, "解析表情信息失败")
	}
	return infos, nil
}

func uploadImageByURL(imageURL string) (string, error) {
	return doUploadImage(ImageUploadRequest{
		Type: "url",
		URL:  imageURL,
	})
}

func uploadImageByBase64(data []byte) (string, error) {
	return doUploadImage(ImageUploadRequest{
		Type: "data",
		Data: base64.StdEncoding.EncodeToString(data),
	})
}

func doUploadImage(req ImageUploadRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", pkgerrors.Wrap(err, "序列化上传请求失败")
	}

	resp, err := httpClient.Post(baseURL+"/image/upload", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", pkgerrors.Wrap(err, "上传图片请求失败")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", parseMemeError(resp.StatusCode, body)
	}

	var imgResp ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&imgResp); err != nil {
		return "", pkgerrors.Wrap(err, "解析上传响应失败")
	}
	return imgResp.ImageID, nil
}

func generateMeme(key string, images []MemeImage, texts []string, options map[string]interface{}) ([]byte, error) {
	if options == nil {
		options = make(map[string]interface{})
	}
	if images == nil {
		images = make([]MemeImage, 0)
	}
	if texts == nil {
		texts = make([]string, 0)
	}

	reqBody := MemeGenerateRequest{
		Images:  images,
		Texts:   texts,
		Options: options,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "序列化生成请求失败")
	}

	resp, err := httpClient.Post(baseURL+"/memes/"+key, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, pkgerrors.Wrap(err, "生成表情请求失败")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, parseMemeError(resp.StatusCode, body)
	}

	var imgResp ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&imgResp); err != nil {
		return nil, pkgerrors.Wrap(err, "解析生成响应失败")
	}
	return getImage(imgResp.ImageID)
}

func parseMemeError(statusCode int, body []byte) error {
	if statusCode == 500 {
		var apiErr MemeAPIError
		if err := json.Unmarshal(body, &apiErr); err == nil {
			return &apiErr
		}
	}
	return pkgerrors.New(fmt.Sprintf("生成表情失败(HTTP %d): %s", statusCode, string(body)))
}

func getImage(imageID string) ([]byte, error) {
	resp, err := httpClient.Get(baseURL + "/image/" + imageID)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "获取图片失败")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, pkgerrors.New(fmt.Sprintf("获取图片失败(HTTP %d): %s", resp.StatusCode, string(body)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "读取图片数据失败")
	}
	if len(data) == 0 {
		return nil, pkgerrors.New("获取到空图片数据")
	}
	return data, nil
}

func renderMemeList() ([]byte, error) {
	reqBody := RenderListRequest{SortBy: "keywords_pinyin"}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "序列化列表请求失败")
	}

	resp, err := httpClient.Post(baseURL+"/tools/render_list", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, pkgerrors.Wrap(err, "渲染列表请求失败")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, pkgerrors.New(fmt.Sprintf("渲染列表失败(HTTP %d): %s", resp.StatusCode, string(body)))
	}

	var imgResp ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&imgResp); err != nil {
		return nil, pkgerrors.Wrap(err, "解析列表响应失败")
	}
	return getImage(imgResp.ImageID)
}
