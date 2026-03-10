package memory

import (
	"context"
	"fkteams/fkevent"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultMaxEntries   = 500
	defaultMinScore     = 1.0
	defaultEvictionDays = 30

	// 小规模全量注入阈值：条目数 ≤ 此值时直接返回全部记忆，不依赖 BM25 搜索
	fullInjectThreshold = 20

	// 提取触发条件
	minNewUserMessages = 2   // 新增至少 2 条用户消息才触发提取
	minNewContentLen   = 300 // 新增消息总字符数至少 300
	extractCooldown    = 3 * time.Minute
)

// Manager 记忆管理器
type Manager struct {
	mu       sync.RWMutex
	storeDir string
	entries  []MemoryEntry
	bm25     *BM25
	llm      LLMClient

	maxEntries   int
	minScore     float64
	evictionDays int

	wg               sync.WaitGroup
	extractedOffsets map[string]int
	lastExtractTime  map[string]time.Time
	dirty            bool
}

// NewManager 创建记忆管理器
func NewManager(workspaceDir string, llmClient LLMClient) *Manager {
	m := &Manager{
		storeDir:         filepath.Join(workspaceDir, "memory"),
		bm25:             &BM25{},
		llm:              llmClient,
		maxEntries:       defaultMaxEntries,
		minScore:         defaultMinScore,
		evictionDays:     defaultEvictionDays,
		extractedOffsets: make(map[string]int),
		lastExtractTime:  make(map[string]time.Time),
	}
	m.load()
	m.rebuildIndex()
	return m
}

// ExtractAndStore 提取记忆并存储
// 内部根据新增消息数量、内容长度和冷却时间智能判断是否触发 LLM 提取
func (m *Manager) ExtractAndStore(ctx context.Context, messages []Message, sessionID string) {
	m.wg.Add(1)
	defer m.wg.Done()

	m.mu.RLock()
	offset := m.extractedOffsets[sessionID]
	lastTime := m.lastExtractTime[sessionID]
	m.mu.RUnlock()

	if offset >= len(messages) {
		return
	}
	newMessages := messages[offset:]

	// 智能触发：检查是否值得调用 LLM 提取
	if !m.shouldExtract(newMessages, lastTime) {
		return
	}

	entries, err := Extract(ctx, newMessages, sessionID, m.llm)
	if err != nil {
		log.Printf("[memory] warn: extract failed: %v\n", err)
		return
	}

	m.mu.Lock()
	m.extractedOffsets[sessionID] = len(messages)
	m.lastExtractTime[sessionID] = time.Now()
	m.mu.Unlock()

	if len(entries) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	added, updated := 0, 0
	for _, entry := range entries {
		action, idx := m.checkDuplicate(entry)
		switch action {
		case actionAdd:
			m.entries = append(m.entries, entry)
			added++
		case actionUpdate:
			// 保留旧条目的命中统计
			entry.HitCount = m.entries[idx].HitCount
			entry.LastHitAt = m.entries[idx].LastHitAt
			m.entries[idx] = entry
			updated++
		case actionSkip:
			// 完全重复，跳过
		}
	}

	if added > 0 || updated > 0 {
		// 容量淘汰
		m.evictIfNeeded()
		m.rebuildIndex()
		if err := m.save(); err != nil {
			log.Printf("[memory] warn: save failed: %v\n", err)
		} else {
			log.Printf("[memory] saved to %s (added: %d, updated: %d, total: %d)\n",
				m.storeDir, added, updated, len(m.entries))
		}
	}
}

// Search 检索记忆
// 当条目总数 ≤ fullInjectThreshold 时直接返回全部，否则走 BM25 搜索并过滤低分结果
func (m *Manager) Search(query string, topK int) []MemoryEntry {
	m.mu.RLock()

	// 小规模全量注入：条目少时直接返回全部，避免纯词法搜索遗漏语义关联
	if len(m.entries) <= fullInjectThreshold {
		result := make([]MemoryEntry, len(m.entries))
		copy(result, m.entries)
		hitIDs := make([]string, len(result))
		for i := range result {
			hitIDs[i] = result[i].ID
		}
		m.mu.RUnlock()
		if len(hitIDs) > 0 {
			go m.updateHitStats(hitIDs)
		}
		return result
	}

	results := m.bm25.Search(query, m.entries, topK)

	// 过滤低于最低分数阈值的结果
	var filtered []SearchResult
	for _, r := range results {
		if r.Score >= m.minScore {
			filtered = append(filtered, r)
		}
	}

	entries := make([]MemoryEntry, len(filtered))
	hitIDs := make([]string, len(filtered))
	for i, r := range filtered {
		entries[i] = *r.Entry
		hitIDs[i] = r.Entry.ID
	}
	m.mu.RUnlock()

	// 异步更新命中统计
	if len(hitIDs) > 0 {
		go m.updateHitStats(hitIDs)
	}

	return entries
}

// Wait 等待所有异步提取任务完成（用于优雅退出）
func (m *Manager) Wait() {
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty {
		if err := m.save(); err != nil {
			log.Printf("[memory] warn: final save failed: %v\n", err)
		}
		m.dirty = false
	}
}

// FlushExtract 强制提取指定会话的剩余消息（退出前调用，跳过触发条件检查）
func (m *Manager) FlushExtract(ctx context.Context, messages []Message, sessionID string) {
	m.mu.RLock()
	offset := m.extractedOffsets[sessionID]
	m.mu.RUnlock()

	if offset >= len(messages) {
		return
	}
	newMessages := messages[offset:]

	// 跳过过短的内容（仍保留基本过滤）
	totalLen := 0
	for _, msg := range newMessages {
		totalLen += len(msg.Content)
	}
	if totalLen < 100 {
		return
	}

	entries, err := Extract(ctx, newMessages, sessionID, m.llm)
	if err != nil {
		log.Printf("[memory] warn: flush extract failed: %v\n", err)
		return
	}

	m.mu.Lock()
	m.extractedOffsets[sessionID] = len(messages)

	added := 0
	for _, entry := range entries {
		action, idx := m.checkDuplicate(entry)
		switch action {
		case actionAdd:
			m.entries = append(m.entries, entry)
			added++
		case actionUpdate:
			entry.HitCount = m.entries[idx].HitCount
			entry.LastHitAt = m.entries[idx].LastHitAt
			m.entries[idx] = entry
			added++
		}
	}

	if added > 0 {
		m.evictIfNeeded()
		m.rebuildIndex()
		if err := m.save(); err != nil {
			log.Printf("[memory] warn: flush save failed: %v\n", err)
		}
	}
	m.mu.Unlock()
}

// List 列出所有记忆条目
func (m *Manager) List() []MemoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MemoryEntry, len(m.entries))
	copy(result, m.entries)
	return result
}

// Delete 删除指定摘要的记忆条目，返回删除数量
func (m *Manager) Delete(summary string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	summary = strings.TrimSpace(summary)
	original := len(m.entries)
	filtered := m.entries[:0]
	for _, e := range m.entries {
		if e.Summary != summary {
			filtered = append(filtered, e)
		}
	}
	m.entries = filtered
	deleted := original - len(m.entries)

	if deleted > 0 {
		m.rebuildIndex()
		if err := m.save(); err != nil {
			log.Printf("[memory] warn: save after delete failed: %v\n", err)
		}
	}
	return deleted
}

// Clear 清空所有记忆
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = []MemoryEntry{}
	m.extractedOffsets = make(map[string]int)
	m.lastExtractTime = make(map[string]time.Time)
	m.rebuildIndex()
	if err := m.save(); err != nil {
		log.Printf("[memory] warn: save after clear failed: %v\n", err)
	}
}

// Count 返回当前记忆条目数
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// ConvertRecorderMessages 将 HistoryRecorder 的消息转为 memory.Message
func ConvertRecorderMessages(recorder *fkevent.HistoryRecorder) []Message {
	recorder.FinalizeCurrent()
	agentMessages := recorder.GetMessages()
	var msgs []Message
	for _, am := range agentMessages {
		role := "assistant"
		if am.AgentName == "用户" || am.AgentName == "user" {
			role = "user"
		}
		content := am.GetTextContent()
		if content != "" {
			msgs = append(msgs, Message{Role: role, Content: content})
		}
	}
	return msgs
}

// ExtractFromRecorder 异步从 HistoryRecorder 提取记忆
func (m *Manager) ExtractFromRecorder(recorder *fkevent.HistoryRecorder, sessionID string) {
	msgs := ConvertRecorderMessages(recorder)
	if len(msgs) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		m.ExtractAndStore(ctx, msgs, sessionID)
	}()
}

// FlushFromRecorder 退出前强制从 HistoryRecorder 提取剩余记忆
func (m *Manager) FlushFromRecorder(recorder *fkevent.HistoryRecorder, sessionID string) {
	msgs := ConvertRecorderMessages(recorder)
	if len(msgs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m.FlushExtract(ctx, msgs, sessionID)
}

// shouldExtract 智能判断是否需要触发 LLM 提取
func (m *Manager) shouldExtract(newMessages []Message, lastTime time.Time) bool {
	// 冷却期内不提取
	if !lastTime.IsZero() && time.Since(lastTime) < extractCooldown {
		return false
	}

	// 统计新增用户消息数和总内容长度
	userMsgCount := 0
	totalLen := 0
	for _, msg := range newMessages {
		totalLen += len(msg.Content)
		if msg.Role == "user" {
			userMsgCount++
		}
	}

	return userMsgCount >= minNewUserMessages && totalLen >= minNewContentLen
}

// updateHitStats 更新命中统计
func (m *Manager) updateHitStats(ids []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	hitSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		hitSet[id] = true
	}

	for i := range m.entries {
		if hitSet[m.entries[i].ID] {
			m.entries[i].HitCount++
			m.entries[i].LastHitAt = &now
			m.dirty = true
		}
	}
}

type duplicateAction int

const (
	actionAdd    duplicateAction = iota // 新条目，直接添加
	actionUpdate                        // 同类型近似条目，更新内容
	actionSkip                          // 完全重复，跳过
)

// checkDuplicate 判断新条目与已有条目的关系
func (m *Manager) checkDuplicate(entry MemoryEntry) (duplicateAction, int) {
	for i, existing := range m.entries {
		if existing.Type != entry.Type {
			continue
		}

		// Summary 完全相同 → 跳过
		if existing.Summary == entry.Summary {
			return actionSkip, i
		}

		// Summary 包含关系，但需要长度比例限制（避免 "喜欢" 包含 "不喜欢" 的误判）
		shorter, longer := existing.Summary, entry.Summary
		if utf8.RuneCountInString(shorter) > utf8.RuneCountInString(longer) {
			shorter, longer = longer, shorter
		}
		shortLen := utf8.RuneCountInString(shorter)
		longLen := utf8.RuneCountInString(longer)
		if shortLen > 0 && longLen > 0 && strings.Contains(longer, shorter) {
			ratio := float64(shortLen) / float64(longLen)
			if ratio > 0.7 {
				// 高相似度包含关系 → 用较新的条目更新
				return actionUpdate, i
			}
		}

		// Tags 重叠比例超过 2/3
		if len(entry.Tags) > 0 && len(existing.Tags) > 0 {
			overlap := 0
			existingTags := make(map[string]bool, len(existing.Tags))
			for _, t := range existing.Tags {
				existingTags[strings.ToLower(t)] = true
			}
			for _, t := range entry.Tags {
				if existingTags[strings.ToLower(t)] {
					overlap++
				}
			}
			minLen := len(entry.Tags)
			if len(existing.Tags) < minLen {
				minLen = len(existing.Tags)
			}
			if float64(overlap)/float64(minLen) > 2.0/3.0 {
				// 高标签重叠 → 用较新的条目更新
				return actionUpdate, i
			}
		}
	}
	return actionAdd, -1
}

// evictIfNeeded 容量淘汰：超过上限时移除低价值条目
func (m *Manager) evictIfNeeded() {
	if len(m.entries) <= m.maxEntries {
		return
	}

	now := time.Now()
	evictionThreshold := now.AddDate(0, 0, -m.evictionDays)

	// 计算每个条目的价值分数
	type scored struct {
		index int
		score float64
	}
	scores := make([]scored, len(m.entries))
	for i, e := range m.entries {
		// 价值 = hitCount * 时间衰减因子
		daysSinceCreation := now.Sub(e.CreatedAt).Hours() / 24
		if daysSinceCreation < 1 {
			daysSinceCreation = 1
		}
		recencyBonus := 1.0
		if e.LastHitAt != nil {
			daysSinceHit := now.Sub(*e.LastHitAt).Hours() / 24
			if daysSinceHit < 7 {
				recencyBonus = 2.0
			}
		}
		score := (float64(e.HitCount) + 1) * recencyBonus / daysSinceCreation
		scores[i] = scored{index: i, score: score}
	}

	// 按价值排序（低价值在前）
	sort.Slice(scores, func(i, j int) bool { return scores[i].score < scores[j].score })

	// 优先淘汰：从未命中 + 超过 evictionDays 的条目
	toRemove := make(map[int]bool)
	for _, s := range scores {
		if len(m.entries)-len(toRemove) <= m.maxEntries {
			break
		}
		e := m.entries[s.index]
		if e.HitCount == 0 && e.CreatedAt.Before(evictionThreshold) {
			toRemove[s.index] = true
		}
	}

	// 如果还需要淘汰更多，按价值分数从低到高移除
	for _, s := range scores {
		if len(m.entries)-len(toRemove) <= m.maxEntries {
			break
		}
		if !toRemove[s.index] {
			toRemove[s.index] = true
		}
	}

	if len(toRemove) > 0 {
		filtered := make([]MemoryEntry, 0, len(m.entries)-len(toRemove))
		for i, e := range m.entries {
			if !toRemove[i] {
				filtered = append(filtered, e)
			}
		}
		m.entries = filtered
		log.Printf("[memory] evicted %d entries, %d remaining\n", len(toRemove), len(m.entries))
	}
}

// rebuildIndex 重建 BM25 索引
func (m *Manager) rebuildIndex() {
	m.bm25 = &BM25{}
	m.bm25.Build(m.entries)
}

func (m *Manager) load() {
	m.entries = loadAllMarkdown(m.storeDir)
}

// save 保存到 Markdown 文件
func (m *Manager) save() error {
	m.dirty = false
	return saveAllMarkdown(m.storeDir, m.entries)
}
