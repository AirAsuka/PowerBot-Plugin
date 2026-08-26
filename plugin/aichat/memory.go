// Package aichat 大模型聊天和Agent
package aichat

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const memorySystemPrefix = "\n\n以下是该用户的历史长期记忆，请在后续对话中自然地参考：\n"

var (
	memberMemoryOnce  sync.Once
	memberMemoryStore *memberMemory
)

// memberMemory 按 QQ 号保存的长期记忆，数据挂载在 data/aichat/memory 下。
type memberMemory struct {
	mu   sync.Mutex
	root string
}

func getMemberMemory() *memberMemory {
	memberMemoryOnce.Do(func() {
		dir := filepath.Join(en.DataFolder(), "memory")
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(err)
		}
		memberMemoryStore = &memberMemory{root: dir}
	})
	return memberMemoryStore
}

func (m *memberMemory) path(userID int64) string {
	return filepath.Join(m.root, strconv.FormatInt(userID, 10)+".txt")
}

// loadText 读取该 QQ 号的全部长期记忆。
func (m *memberMemory) loadText(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := os.ReadFile(m.path(userID))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// append 向该 QQ 号追加一条长期记忆。
func (m *memberMemory) append(userID int64, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	f, err := os.OpenFile(m.path(userID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(text + "\n"); err != nil {
		return err
	}
	return nil
}

// Save 实现 goba.MemoryStorage，按当前对话对应的 QQ 号保存长期记忆。
func (m *memberMemory) Save(grp int64, text string) error {
	return m.append(agentMemoryUser(grp), text)
}

// Load 实现 goba.MemoryStorage，读取当前对话对应 QQ 号的长期记忆。
func (m *memberMemory) Load(grp int64) []string {
	mem := m.loadText(agentMemoryUser(grp))
	if mem == "" {
		return nil
	}
	return []string{mem}
}

// clear 清空该 QQ 号的长期记忆。
func (m *memberMemory) clear(userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := os.Remove(m.path(userID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// memorySystemText 生成可拼接到系统提示词中的长期记忆文本。
func memorySystemText(userID int64) string {
	mem := getMemberMemory().loadText(userID)
	if mem == "" {
		return ""
	}
	return memorySystemPrefix + mem
}
