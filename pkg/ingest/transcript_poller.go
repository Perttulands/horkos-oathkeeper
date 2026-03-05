package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultPollInterval = 3 * time.Second
	defaultMaxSeen      = 8192
)

// Message is one parsed transcript payload forwarded to analysis.
type Message struct {
	SessionKey string
	Role       string
	Text       string
	MessageID  string
}

// HandleFunc receives parsed transcript messages.
type HandleFunc func(Message) error

// TranscriptPoller tails transcript.jsonl files under a transcript root.
// It starts from EOF for existing files to avoid replaying historical content.
type TranscriptPoller struct {
	root     string
	interval time.Duration
	handle   HandleFunc

	mu      sync.Mutex
	offsets map[string]int64
	seen    map[uint64]struct{}
	order   []uint64
	maxSeen int

	stopCh    chan struct{}
	doneCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewTranscriptPoller creates a transcript poller.
func NewTranscriptPoller(root string, interval time.Duration, handle HandleFunc) *TranscriptPoller {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return &TranscriptPoller{
		root:     strings.TrimSpace(root),
		interval: interval,
		handle:   handle,
		offsets:  make(map[string]int64),
		seen:     make(map[uint64]struct{}),
		maxSeen:  defaultMaxSeen,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start begins polling in a background goroutine.
func (p *TranscriptPoller) Start() {
	p.startOnce.Do(func() {
		go p.run()
	})
}

// Stop halts the poller and waits for goroutine exit.
func (p *TranscriptPoller) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
		<-p.doneCh
	})
}

func (p *TranscriptPoller) run() {
	defer close(p.doneCh)

	_ = p.scanOnce()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = p.scanOnce()
		case <-p.stopCh:
			return
		}
	}
}

func (p *TranscriptPoller) scanOnce() error {
	if p.root == "" {
		return nil
	}

	files, err := discoverTranscriptFiles(p.root)
	if err != nil {
		return fmt.Errorf("discover transcript files: %w", err)
	}
	for _, path := range files {
		if err := p.processFile(path); err != nil {
			continue
		}
	}
	return nil
}

func discoverTranscriptFiles(root string) ([]string, error) {
	files := make([]string, 0, 16)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "transcript.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk transcript root: %w", err)
	}
	return files, nil
}

func (p *TranscriptPoller) processFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat transcript: %w", err)
	}
	size := info.Size()

	p.mu.Lock()
	offset, known := p.offsets[path]
	if !known {
		p.offsets[path] = size
		p.mu.Unlock()
		return nil
	}
	if size < offset {
		offset = 0
	}
	p.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek transcript: %w", err)
	}

	r := bufio.NewReader(f)
	pos := offset

	for {
		line, readErr := r.ReadBytes('\n')
		if len(line) > 0 {
			pos += int64(len(line))
			p.processLine(path, line)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read transcript: %w", readErr)
		}
	}

	p.mu.Lock()
	p.offsets[path] = pos
	p.mu.Unlock()
	return nil
}

func (p *TranscriptPoller) processLine(path string, raw []byte) {
	line := strings.TrimSpace(string(raw))
	if line == "" {
		return
	}

	role, texts, messageID := parseTranscriptLine([]byte(line))
	if strings.ToLower(strings.TrimSpace(role)) != "assistant" || len(texts) == 0 {
		return
	}

	sessionKey := filepath.Base(filepath.Dir(path))
	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		fingerprint := buildFingerprint(sessionKey, messageID, text, line)
		if !p.markSeen(fingerprint) {
			continue
		}

		if p.handle != nil {
			_ = p.handle(Message{
				SessionKey: sessionKey,
				Role:       "assistant",
				Text:       text,
				MessageID:  messageID,
			})
		}
	}
}

func buildFingerprint(sessionKey string, messageID string, text string, fallback string) uint64 {
	material := strings.TrimSpace(messageID)
	if material == "" {
		material = strings.TrimSpace(fallback)
	}

	h := fnv.New64a()
	if _, err := h.Write([]byte(strings.ToLower(strings.TrimSpace(sessionKey)))); err != nil {
		return 0
	}
	if _, err := h.Write([]byte("|")); err != nil {
		return 0
	}
	if _, err := h.Write([]byte(strings.ToLower(material))); err != nil {
		return 0
	}
	if _, err := h.Write([]byte("|")); err != nil {
		return 0
	}
	if _, err := h.Write([]byte(strings.TrimSpace(text))); err != nil {
		return 0
	}
	return h.Sum64()
}

func (p *TranscriptPoller) markSeen(fingerprint uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.seen[fingerprint]; ok {
		return false
	}
	p.seen[fingerprint] = struct{}{}
	p.order = append(p.order, fingerprint)

	if len(p.order) > p.maxSeen {
		prune := len(p.order) / 2
		for i := 0; i < prune; i++ {
			delete(p.seen, p.order[i])
		}
		p.order = append([]uint64(nil), p.order[prune:]...)
	}
	return true
}

type transcriptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openClawContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type openClawMessage struct {
	Role    string                 `json:"role"`
	Content []openClawContentBlock `json:"content"`
}

type openClawLine struct {
	ID      string          `json:"id"`
	Message openClawMessage `json:"message"`
}

func parseTranscriptLine(data []byte) (role string, texts []string, messageID string) {
	var oc openClawLine
	if err := json.Unmarshal(data, &oc); err == nil && oc.Message.Role != "" {
		for _, block := range oc.Message.Content {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				texts = append(texts, block.Text)
			}
		}
		return oc.Message.Role, texts, oc.ID
	}

	var flat transcriptMessage
	if err := json.Unmarshal(data, &flat); err == nil && flat.Role != "" {
		if strings.TrimSpace(flat.Content) != "" {
			texts = append(texts, flat.Content)
		}
		return flat.Role, texts, ""
	}

	return "", nil, ""
}
