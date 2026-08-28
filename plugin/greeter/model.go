// Package greeter 入群欢迎/退群欢送
package greeter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/FloatTech/floatbox/file"
	"github.com/sirupsen/logrus"
)

const (
	defaultWelcome  = "欢迎 {at} 加入本群~"
	defaultFarewell = "{nickname}({uid}) 离开了我们..."
)

type groupConfig struct {
	Welcome  string `json:"welcome"`
	Farewell string `json:"farewell"`
}

var (
	cfgmu   sync.RWMutex
	cfgs    = make(map[string]*groupConfig)
	cfgpath = file.BOTPATH + "/" + engine.DataFolder() + "config.json"
)

func init() {
	data, err := os.ReadFile(cfgpath)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.Warnf("[greeter] 读取配置文件失败: %v", err)
		}
		return
	}
	err = json.Unmarshal(data, &cfgs)
	if err != nil {
		logrus.Warnf("[greeter] 解析配置文件失败: %v", err)
	}
}

// load 获取指定群的配置, 不存在则返回默认配置
func load(gid int64) *groupConfig {
	key := strconv.FormatInt(gid, 10)
	cfgmu.RLock()
	c, ok := cfgs[key]
	cfgmu.RUnlock()
	if ok {
		return c
	}
	cfgmu.Lock()
	defer cfgmu.Unlock()
	if c, ok := cfgs[key]; ok {
		return c
	}
	c = &groupConfig{Welcome: defaultWelcome, Farewell: defaultFarewell}
	cfgs[key] = c
	return c
}

// save 保存指定群的配置并写盘
func save(gid int64, c *groupConfig) error {
	key := strconv.FormatInt(gid, 10)
	cfgmu.Lock()
	defer cfgmu.Unlock()
	cfgs[key] = c
	if err := os.MkdirAll(filepath.Dir(cfgpath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfgs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgpath, data, 0644)
}
