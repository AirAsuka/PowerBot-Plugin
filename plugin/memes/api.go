package memes

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ===================== 配置 =====================

var baseURL = "http://127.0.0.1:2233"

var httpClient = &http.Client{Timeout: 60 * time.Second}

// ===================== 通用响应结构 =====================

type imageResponse struct {
	ImageID string `json:"image_id"`
}

type imagesResponse struct {
	ImageIDs []string `json:"image_ids"`
}

type errorResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ===================== MemeInfo 相关结构体 =====================

// ParserFlags 选项解析标记
type ParserFlags struct {
	Short        bool     `json:"short"`
	Long         bool     `json:"long"`
	ShortAliases []string `json:"short_aliases"`
	LongAliases  []string `json:"long_aliases"`
}

// MemeOption 表情选项（统一结构）
type MemeOption struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // "boolean", "string", "integer", "float"
	Description *string     `json:"description"`
	ParserFlags ParserFlags `json:"parser_flags"`
	Default     interface{} `json:"default"`
	Choices     []string    `json:"choices,omitempty"`
	Minimum     interface{} `json:"minimum,omitempty"`
	Maximum     interface{} `json:"maximum,omitempty"`
}

// MemeParams 表情参数信息
type MemeParams struct {
	MinImages    int          `json:"min_images"`
	MaxImages    int          `json:"max_images"`
	MinTexts     int          `json:"min_texts"`
	MaxTexts     int          `json:"max_texts"`
	DefaultTexts []string     `json:"default_texts"`
	Options      []MemeOption `json:"options"`
}

// MemeShortcut 表情快捷指令
type MemeShortcut struct {
	Pattern   string                 `json:"pattern"`
	Humanized *string                `json:"humanized"`
	Names     []string               `json:"names"`
	Texts     []string               `json:"texts"`
	Options   map[string]interface{} `json:"options"`
}

// MemeInfo 表情信息
type MemeInfo struct {
	Key          string         `json:"key"`
	Params       MemeParams     `json:"params"`
	Keywords     []string       `json:"keywords"`
	Shortcuts    []MemeShortcut `json:"shortcuts"`
	Tags         []string       `json:"tags"`
	DateCreated  string         `json:"date_created"`
	DateModified string         `json:"date_modified"`
}

// MemeGeneratorError 表情生成器错误
type MemeGeneratorError struct {
	Code    int
	Message string
	Detail  string
}

func (e *MemeGeneratorError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// ===================== 基础请求函数 =====================

func doRequest(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == 200 {
		return respData, nil
	}

	if resp.StatusCode == 500 {
		var errResp errorResponse
		if err := json.Unmarshal(respData, &errResp); err == nil {
			detail := ""
			var dataMap map[string]interface{}
			if json.Unmarshal(errResp.Data, &dataMap) == nil {
				if e, ok := dataMap["error"]; ok {
					detail = fmt.Sprintf("%v", e)
				} else if f, ok := dataMap["feedback"]; ok {
					detail = fmt.Sprintf("%v", f)
				}
			}
			return nil, &MemeGeneratorError{
				Code:    errResp.Code,
				Message: errResp.Message,
				Detail:  detail,
			}
		}
	}

	return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respData))
}

func doGet(path string) ([]byte, error) {
	return doRequest("GET", path, nil)
}

func doPost(path string, body interface{}) ([]byte, error) {
	return doRequest("POST", path, body)
}

// ===================== API 函数 =====================

// uploadImage 上传图片并返回 image_id
func uploadImage(imageData []byte) (string, error) {
	payload := map[string]string{
		"type": "data",
		"data": base64.StdEncoding.EncodeToString(imageData),
	}
	data, err := doPost("/image/upload", payload)
	if err != nil {
		return "", err
	}
	var resp imageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}
	return resp.ImageID, nil
}

// uploadImageByURL 通过 URL 上传图片并返回 image_id
func uploadImageByURL(imageURL string) (string, error) {
	payload := map[string]string{
		"type": "url",
		"url":  imageURL,
	}
	data, err := doPost("/image/upload", payload)
	if err != nil {
		return "", err
	}
	var resp imageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse upload response: %w", err)
	}
	return resp.ImageID, nil
}

// getImage 通过 image_id 获取图片数据
func getImage(imageID string) ([]byte, error) {
	return doGet("/image/" + imageID)
}

// getVersion 获取 meme-generator-rs 版本号
func getVersion() (string, error) {
	data, err := doGet("/meme/version")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getMemeKeys 获取所有表情 key 列表
func getMemeKeys() ([]string, error) {
	data, err := doGet("/meme/keys")
	if err != nil {
		return nil, err
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parse keys: %w", err)
	}
	return keys, nil
}

// getMemeInfos 获取所有表情信息列表
func getMemeInfos() ([]MemeInfo, error) {
	data, err := doGet("/meme/infos")
	if err != nil {
		return nil, err
	}
	var infos []MemeInfo
	if err := json.Unmarshal(data, &infos); err != nil {
		return nil, fmt.Errorf("parse infos: %w", err)
	}
	return infos, nil
}

// getMemeInfo 获取单个表情信息
func getMemeInfo(memeKey string) (*MemeInfo, error) {
	data, err := doGet("/memes/" + memeKey + "/info")
	if err != nil {
		return nil, err
	}
	var info MemeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse info: %w", err)
	}
	return &info, nil
}

// searchMemes 搜索表情
func searchMemes(query string, includeTags bool) ([]string, error) {
	params := url.Values{}
	params.Set("query", query)
	if includeTags {
		params.Set("include_tags", "true")
	}
	data, err := doGet("/meme/search?" + params.Encode())
	if err != nil {
		return nil, err
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parse search result: %w", err)
	}
	return keys, nil
}

// generateMemePreview 生成表情预览
func generateMemePreview(memeKey string) ([]byte, error) {
	data, err := doGet("/memes/" + memeKey + "/preview")
	if err != nil {
		return nil, err
	}
	var resp imageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse preview response: %w", err)
	}
	return getImage(resp.ImageID)
}

// MemeImage 表情图片参数
type MemeImage struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// generateMeme 生成表情
func generateMeme(memeKey string, images []MemeImage, texts []string, options map[string]interface{}) ([]byte, error) {
	if texts == nil {
		texts = []string{}
	}
	if options == nil {
		options = map[string]interface{}{}
	}
	payload := map[string]interface{}{
		"images":  images,
		"texts":   texts,
		"options": options,
	}
	data, err := doPost("/memes/"+memeKey, payload)
	if err != nil {
		return nil, err
	}
	var resp imageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse generate response: %w", err)
	}
	return getImage(resp.ImageID)
}

// renderMemeList 渲染表情列表图片
func renderMemeList() ([]byte, error) {
	payload := map[string]interface{}{
		"meme_properties":   map[string]interface{}{},
		"exclude_memes":     []string{},
		"sort_by":           "keywords_pinyin",
		"sort_reverse":      false,
		"text_template":     "{index}. {keywords}",
		"add_category_icon": true,
	}
	data, err := doPost("/tools/render_list", payload)
	if err != nil {
		return nil, err
	}
	var resp imageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse render list response: %w", err)
	}
	return getImage(resp.ImageID)
}
