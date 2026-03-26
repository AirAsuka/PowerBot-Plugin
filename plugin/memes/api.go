package memes

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// fetchMemeInfos 获取所有表情信息列表
func fetchMemeInfos() ([]MemeInfo, error) {
	url := baseURL + "/meme/infos"
	logrus.Infof("[memes] 请求 API: GET %s", url)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取表情信息失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取表情信息失败(HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var infos []MemeInfo
	err = json.NewDecoder(resp.Body).Decode(&infos)
	if err != nil {
		return nil, fmt.Errorf("解析表情信息失败: %w", err)
	}
	logrus.Infof("[memes] 成功获取 %d 个表情信息", len(infos))
	return infos, nil
}

// uploadImageByURL 通过URL上传图片
func uploadImageByURL(imageURL string) (string, error) {
	reqBody := ImageUploadRequest{
		Type: "url",
		URL:  imageURL,
	}
	return doUploadImage(reqBody)
}

// uploadImageByBase64 通过Base64上传图片
func uploadImageByBase64(data []byte) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(data)
	reqBody := ImageUploadRequest{
		Type: "data",
		Data: b64,
	}
	return doUploadImage(reqBody)
}

// doUploadImage 执行图片上传
func doUploadImage(req ImageUploadRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("序列化上传请求失败: %w", err)
	}
	logrus.Debugf("[memes] 上传图片, type=%s", req.Type)
	resp, err := httpClient.Post(baseURL+"/image/upload", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("上传图片请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("上传图片失败(HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var imgResp ImageResponse
	err = json.NewDecoder(resp.Body).Decode(&imgResp)
	if err != nil {
		return "", fmt.Errorf("解析上传响应失败: %w", err)
	}
	logrus.Debugf("[memes] 上传图片成功, id=%s", imgResp.ImageID)
	return imgResp.ImageID, nil
}

// generateMeme 生成表情
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
		return nil, fmt.Errorf("序列化生成请求失败: %w", err)
	}
	logrus.Infof("[memes] 生成表情 key=%s images=%d texts=%v body=%s", key, len(images), texts, string(body))
	resp, err := httpClient.Post(baseURL+"/memes/"+key, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("生成表情请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("生成表情失败(HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var imgResp ImageResponse
	err = json.NewDecoder(resp.Body).Decode(&imgResp)
	if err != nil {
		return nil, fmt.Errorf("解析生成响应失败: %w", err)
	}
	logrus.Debugf("[memes] 生成成功, image_id=%s", imgResp.ImageID)
	return getImage(imgResp.ImageID)
}

// getImage 通过图片ID获取图片数据
func getImage(imageID string) ([]byte, error) {
	url := baseURL + "/image/" + imageID
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取图片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取图片失败(HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取图片数据失败: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("获取到空图片数据")
	}
	return data, nil
}

// renderMemeList 渲染表情列表图片
func renderMemeList() ([]byte, error) {
	reqBody := RenderListRequest{
		SortBy: "keywords_pinyin",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化列表请求失败: %w", err)
	}
	logrus.Infof("[memes] 请求渲染表情列表")
	resp, err := httpClient.Post(baseURL+"/tools/render_list", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("渲染列表请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("渲染列表失败(HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var imgResp ImageResponse
	err = json.NewDecoder(resp.Body).Decode(&imgResp)
	if err != nil {
		return nil, fmt.Errorf("解析列表响应失败: %w", err)
	}
	return getImage(imgResp.ImageID)
}
