package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Budget struct {
	Path  string
	Limit int64
	Now   func() time.Time

	mu    sync.Mutex
	day   string
	spent int64
}

type usage struct {
	Day    string `json:"day"`
	Tokens int64  `json:"tokens"`
}

func (b *Budget) today() string {
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	return now().UTC().Format("2006-01-02")
}

func (b *Budget) rollLocked() {
	today := b.today()
	if b.day == today {
		return
	}
	b.day, b.spent = today, 0
	if b.Path == "" {
		return
	}
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return
	}
	var u usage
	if json.Unmarshal(data, &u) == nil && u.Day == today {
		b.spent = u.Tokens
	}
}

func (b *Budget) Add(tokens int) {
	if b == nil || tokens <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	b.spent += int64(tokens)
	b.saveLocked()
}

func (b *Budget) saveLocked() {
	if b.Path == "" {
		return
	}
	data, err := json.Marshal(usage{Day: b.day, Tokens: b.spent})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(b.Path), 0o750); err != nil {
		return
	}
	_ = os.WriteFile(b.Path, data, 0o600)
}

func (b *Budget) Spent() int64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	return b.spent
}

func (b *Budget) Exhausted() bool {
	if b == nil || b.Limit <= 0 {
		return false
	}
	return b.Spent() >= b.Limit
}
