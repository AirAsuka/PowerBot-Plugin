package amongus

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

const (
	// toueWebBase TOUE-Web API 地址
	toueWebBase = "https://api.toue.mxyx.club"
)

// tokenExpireMargin token 提前判定过期的余量，避免边界时刻请求被 401
const tokenExpireMargin = 5 * time.Minute

// errPlayerCodeTaken 目标 PlayerCode 已被其他网页账号绑定
var errPlayerCodeTaken = errors.New("该玩家已在网页端绑定账号，无法查询")

var (
	// webRankingsMu 全局锁：整理绑定 + 查排行整体串行，防止并发查询污染绑定关系
	webRankingsMu sync.Mutex
	// webSess TOUE-Web 登录态（进程内缓存）
	webSess toueWebSession
	// webHTTPClient TOUE-Web 请求客户端
	webHTTPClient = &http.Client{Timeout: 30 * time.Second}
	// webAccount 一次性从 .env / 环境变量加载 TOUE-Web 账号
	webAccount = sync.OnceValues(loadWebAccount)
)

// toueWebSession 登录态缓存
type toueWebSession struct {
	token     string
	expiresAt time.Time
}

// queryPlayerRankingsViaWeb 通过切换 TOUE-Web 账号绑定查询任意 PlayerCode 的排行（全局串行）
func queryPlayerRankingsViaWeb(playerCode string) (*playerRankings, error) {
	webRankingsMu.Lock()
	defer webRankingsMu.Unlock()
	if err := ensureBinding(playerCode); err != nil {
		return nil, err
	}
	return fetchWebRankings()
}

// ensureBinding 将当前账号的绑定整理为只绑定目标 code
func ensureBinding(target string) error {
	codes, err := currentBoundCodes()
	if err != nil {
		return err
	}
	bound := false
	for _, c := range codes {
		if c == target {
			bound = true
			break
		}
	}
	if bound {
		// 目标已绑定且是唯一绑定：直接查，零成本
		if len(codes) == 1 {
			return nil
		}
		// 目标已绑定但还绑了别的：解绑其他，否则排行是多 code 合并数据
		for _, c := range codes {
			if c != target {
				if err := unbindPlayerCode(c); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// 目标未绑定：解绑所有现有绑定（服务端限制最多绑定 2 个），再走绑定 + 验证流程
	for _, c := range codes {
		if err := unbindPlayerCode(c); err != nil {
			return err
		}
	}
	verificationCode, err := bindPlayerCode(target)
	if err != nil {
		return err
	}
	return verifyPlayerCodeBinding(target, verificationCode)
}

// currentBoundCodes 查询当前账号已绑定的 code 列表
func currentBoundCodes() ([]string, error) {
	result, err := authedRequest(http.MethodGet, "/api/user/dashboard", nil)
	if err != nil {
		return nil, err
	}
	if !result.Get("success").Bool() {
		return nil, errors.New(errorMessageFromResult(result, "查询绑定列表失败"))
	}
	codes := make([]string, 0, 2)
	for _, item := range result.Get("data.playerCodes").Array() {
		if code := item.Get("player_code").String(); code != "" {
			codes = append(codes, code)
		}
	}
	return codes, nil
}

// unbindPlayerCode 解绑指定 code（未绑定视为成功，保证切换流程幂等）
func unbindPlayerCode(code string) error {
	body, _ := json.Marshal(map[string]string{"player_code": code})
	result, err := authedRequest(http.MethodPost, "/api/auth/unbind-player-code", body)
	if err != nil {
		return err
	}
	if !result.Get("success").Bool() && result.Get("error_code").String() != "NOT_BOUND" {
		return errors.New(errorMessageFromResult(result, "解绑失败"))
	}
	return nil
}

// bindPlayerCode 发起绑定，响应里直接返回验证码；撞绑返回 errPlayerCodeTaken
func bindPlayerCode(code string) (string, error) {
	body, _ := json.Marshal(map[string]string{"player_code": code})
	result, err := authedRequest(http.MethodPost, "/api/auth/bind-player-code", body)
	if err != nil {
		return "", err
	}
	if !result.Get("success").Bool() {
		if result.Get("error_code").String() == "PLAYER_CODE_TAKEN" {
			return "", errPlayerCodeTaken
		}
		return "", errors.New(errorMessageFromResult(result, "绑定失败"))
	}
	return result.Get("verification_code").String(), nil
}

// verifyPlayerCodeBinding 用验证码完成绑定
func verifyPlayerCodeBinding(code, verificationCode string) error {
	// 服务端在已绑定时返回空验证码，无需再走验证
	if verificationCode == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"player_code": code, "verification_code": verificationCode})
	result, err := authedRequest(http.MethodPost, "/api/auth/verify-playercode-binding", body)
	if err != nil {
		return err
	}
	if !result.Get("success").Bool() {
		return errors.New(errorMessageFromResult(result, "验证绑定失败"))
	}
	return nil
}

// fetchWebRankings 查询当前绑定 code 的排行数据
func fetchWebRankings() (*playerRankings, error) {
	result, err := authedRequest(http.MethodGet, "/api/user/rankings", nil)
	if err != nil {
		return nil, err
	}
	if !result.Get("success").Bool() {
		return nil, errors.New(errorMessageFromResult(result, "查询排行失败"))
	}
	rankings := &playerRankings{}
	for _, item := range result.Get("data.topVictims").Array() {
		rankings.TopVictims = append(rankings.TopVictims, rankingEntry{
			Name:  item.Get("name").String(),
			Count: item.Get("count").Int(),
		})
	}
	for _, item := range result.Get("data.topKillers").Array() {
		rankings.TopKillers = append(rankings.TopKillers, rankingEntry{
			Name:  item.Get("name").String(),
			Count: item.Get("count").Int(),
		})
	}
	deathRoles := make([]rankingEntry, 0, 20)
	for _, item := range result.Get("data.deathRoles").Array() {
		deathRoles = append(deathRoles, rankingEntry{
			Name:      item.Get("name").String(),
			Total:     item.Get("total").Int(),
			Deaths:    item.Get("deaths").Int(),
			DeathRate: item.Get("deathRate").Float(),
		})
	}
	// 职业按中文名合并重算死亡率（沿用本地口径）
	rankings.DeathRoles = mergeDeathRolesByDisplayName(deathRoles)
	return rankings, nil
}

// login 登录 TOUE-Web 并缓存 token（会踢掉网页端会话，靠 token 缓存只在过期时发生）
func (s *toueWebSession) login() error {
	email, password := webAccount()
	if email == "" || password == "" {
		return errors.New("未配置 TOUE-Web 登录账号（请在 .env 中设置 TOUE_WEB_EMAIL / TOUE_WEB_PASSWORD）")
	}
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	data, _, err := webRequest(http.MethodPost, toueWebBase+"/api/auth/login", "", body)
	if err != nil {
		return err
	}
	result := gjson.ParseBytes(data)
	if !result.Get("success").Bool() {
		return errors.New(errorMessageFromResult(result, "登录失败"))
	}
	token := result.Get("token").String()
	expiresAt, err := jwtExpiresAt(token)
	if err != nil {
		return err
	}
	s.token = token
	s.expiresAt = expiresAt
	return nil
}

// ensureToken token 未过期直接用，否则重新登录
func (s *toueWebSession) ensureToken() error {
	if s.token != "" && time.Now().Add(tokenExpireMargin).Before(s.expiresAt) {
		return nil
	}
	return s.login()
}

// authedRequest 带登录态请求；401（过期/会话失效）时自动重新登录并重试一次
func authedRequest(method, path string, jsonBody []byte) (gjson.Result, error) {
	data, status, err := authedRequestOnce(method, path, jsonBody)
	if err == nil && status == http.StatusUnauthorized {
		webSess.token = "" // 丢弃失效 token，强制重新登录
		data, status, err = authedRequestOnce(method, path, jsonBody)
	}
	if err != nil {
		return gjson.Result{}, err
	}
	result := gjson.ParseBytes(data)
	if status == http.StatusUnauthorized {
		return result, errors.New(errorMessageFromResult(result, "登录态失效，请稍后重试"))
	}
	return result, nil
}

func authedRequestOnce(method, path string, jsonBody []byte) ([]byte, int, error) {
	if err := webSess.ensureToken(); err != nil {
		return nil, 0, err
	}
	return webRequest(method, toueWebBase+path, webSess.token, jsonBody)
}

// webRequest 发送 TOUE-Web 请求，token 非空时带 Bearer 头
func webRequest(method, fullURL, token string, jsonBody []byte) ([]byte, int, error) {
	var body io.Reader
	if jsonBody != nil {
		body = bytes.NewReader(jsonBody)
	}
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, 0, err
	}
	if jsonBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := webHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return data, resp.StatusCode, nil
}

// jwtExpiresAt 解析 JWT payload 的 exp（不验签，仅用于本地过期预判）
func jwtExpiresAt(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("token 格式不正确")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, err
	}
	exp := gjson.GetBytes(payload, "exp").Int()
	if exp == 0 {
		return time.Time{}, errors.New("token 缺少 exp")
	}
	return time.Unix(exp, 0), nil
}

// loadWebAccount 读取机器人专用 TOUE-Web 账号：真实环境变量优先，其次 .env 文件
func loadWebAccount() (email, password string) {
	env := make(map[string]string, 2)
	if data, err := os.ReadFile(".env"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			// 去掉成对的引号
			if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
				value = value[1 : len(value)-1]
			}
			env[key] = value
		}
	}
	get := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return env[key]
	}
	return get("TOUE_WEB_EMAIL"), get("TOUE_WEB_PASSWORD")
}
