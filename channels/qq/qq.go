package qq

import (
	"context"
	"fkteams/channels"
	"fkteams/log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/token"
)

func init() {
	channels.RegisterFactory("qq", NewChannel)
}

// chatState 保存每个会话的最近消息 ID 和序号（用于被动回复）
type chatState struct {
	mu       sync.Mutex
	msgID    string
	msgSeq   atomic.Uint32
	lastSeen time.Time
}

// Channel QQ 机器人通道
type Channel struct {
	appID     string
	appSecret string
	sandbox   bool

	api     openapi.OpenAPI
	handler channels.MessageHandler
	running atomic.Bool
	cancel  context.CancelFunc

	// 消息去重（TTL 5 分钟）
	seen   map[string]time.Time
	seenMu sync.Mutex

	// 每个会话的状态
	states   map[string]*chatState
	statesMu sync.RWMutex
}

// NewChannel 创建 QQ 通道实例
func NewChannel(cfg channels.ChannelConfig, handler channels.MessageHandler) (channels.Channel, error) {
	return &Channel{
		appID:     cfg.Extra["app_id"],
		appSecret: cfg.Extra["app_secret"],
		sandbox:   cfg.Extra["sandbox"] == "true",
		handler:   handler,
		seen:      make(map[string]time.Time),
		states:    make(map[string]*chatState),
	}, nil
}

func (c *Channel) Name() string    { return "qq" }
func (c *Channel) IsRunning() bool { return c.running.Load() }

// Start 启动 QQ 机器人 WebSocket 连接
func (c *Channel) Start(ctx context.Context) error {
	tokenSource := token.NewQQBotTokenSource(
		&token.QQBotCredentials{
			AppID:     c.appID,
			AppSecret: c.appSecret,
		},
	)

	if err := token.StartRefreshAccessToken(ctx, tokenSource); err != nil {
		return err
	}

	if c.sandbox {
		c.api = botgo.NewSandboxOpenAPI(c.appID, tokenSource).WithTimeout(10 * time.Second)
	} else {
		c.api = botgo.NewOpenAPI(c.appID, tokenSource).WithTimeout(10 * time.Second)
	}

	intent := event.RegisterHandlers(
		c.c2cMessageHandler(),
		c.groupATMessageHandler(),
	)

	wsInfo, err := c.api.WS(ctx, nil, "")
	if err != nil {
		return err
	}

	wsCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.running.Store(true)

	go func() {
		defer c.running.Store(false)
		if err := botgo.NewSessionManager().Start(wsInfo, tokenSource, &intent); err != nil {
			log.Printf("[qq] session manager exited: %v", err)
		}
		cancel()
	}()

	// 定期清理过期的去重记录
	go c.cleanupSeen(wsCtx)

	log.Printf("[qq] QQ bot started (appID=%s, sandbox=%v)", c.appID, c.sandbox)
	return nil
}

// Stop 停止 QQ 机器人
func (c *Channel) Stop(_ context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.running.Store(false)
	log.Printf("[qq] QQ bot stopped")
	return nil
}

// Send 向指定会话发送消息（支持文本和多媒体）
func (c *Channel) Send(ctx context.Context, chatID string, msg channels.Message) error {
	isGroup := strings.HasPrefix(chatID, "group:")
	targetID := strings.TrimPrefix(chatID, "group:")
	targetID = strings.TrimPrefix(targetID, "c2c:")

	state := c.getState(chatID)
	seq := state.msgSeq.Add(1)

	// 富媒体消息（图片、语音、视频、文件）
	if msg.Type != channels.MsgText && len(msg.Attachments) > 0 {
		for _, att := range msg.Attachments {
			richMsg := &dto.RichMediaMessage{
				FileType:   qqFileType(att.Type),
				URL:        att.URL,
				SrvSendMsg: true,
				MsgSeq:     int64(seq),
			}
			if isGroup {
				_, err := c.api.PostGroupMessage(ctx, targetID, richMsg)
				if err != nil {
					return err
				}
			} else {
				_, err := c.api.PostC2CMessage(ctx, targetID, richMsg)
				if err != nil {
					return err
				}
			}
			seq = state.msgSeq.Add(1)
		}
		// 如果富媒体消息同时附带文本，继续发送文本
		if msg.Content == "" {
			return nil
		}
	}

	// 文本消息
	state.mu.Lock()
	msgID := state.msgID
	state.mu.Unlock()
	textMsg := &dto.MessageToCreate{
		Content: msg.Content,
		MsgType: dto.TextMsg,
		MsgID:   msgID,
		MsgSeq:  uint32(seq),
	}

	if isGroup {
		_, err := c.api.PostGroupMessage(ctx, targetID, textMsg)
		return err
	}
	_, err := c.api.PostC2CMessage(ctx, targetID, textMsg)
	return err
}

// qqFileType 将通用消息类型映射为 QQ 富媒体文件类型
func qqFileType(t channels.MessageType) uint64 {
	switch t {
	case channels.MsgImage:
		return 1
	case channels.MsgVideo:
		return 2
	case channels.MsgAudio:
		return 3
	default:
		return 1
	}
}

// c2cMessageHandler 处理 C2C（私聊）消息
func (c *Channel) c2cMessageHandler() event.C2CMessageEventHandler {
	return func(ev *dto.WSPayload, data *dto.WSC2CMessageData) error {
		msg := (*dto.Message)(data)
		if c.isDuplicate(msg.ID) {
			return nil
		}

		chatID := "c2c:" + msg.Author.ID
		c.updateState(chatID, msg.ID)

		content := strings.TrimSpace(msg.Content)
		attachments := extractAttachments(msg)
		if content == "" && len(attachments) == 0 {
			return nil
		}

		inMsg := channels.Message{Content: content, Attachments: attachments}
		if len(attachments) > 0 {
			inMsg.Type = attachments[0].Type
		}

		ctx := channels.WithChannelName(context.Background(), "qq")
		go c.handler(ctx, chatID, msg.Author.ID, inMsg, false)
		return nil
	}
}

// groupATMessageHandler 处理群 @机器人 消息
func (c *Channel) groupATMessageHandler() event.GroupATMessageEventHandler {
	return func(ev *dto.WSPayload, data *dto.WSGroupATMessageData) error {
		msg := (*dto.Message)(data)
		if c.isDuplicate(msg.ID) {
			return nil
		}

		chatID := "group:" + msg.GroupID
		c.updateState(chatID, msg.ID)

		content := strings.TrimSpace(msg.Content)
		attachments := extractAttachments(msg)
		if content == "" && len(attachments) == 0 {
			return nil
		}

		inMsg := channels.Message{Content: content, Attachments: attachments}
		if len(attachments) > 0 {
			inMsg.Type = attachments[0].Type
		}

		ctx := channels.WithChannelName(context.Background(), "qq")
		go c.handler(ctx, chatID, msg.Author.ID, inMsg, true)
		return nil
	}
}

// extractAttachments 从 QQ 消息中提取附件
func extractAttachments(msg *dto.Message) []channels.Attachment {
	if len(msg.Attachments) == 0 {
		return nil
	}
	var atts []channels.Attachment
	for _, a := range msg.Attachments {
		t := guessAttachmentType(a.URL, a.FileName)
		atts = append(atts, channels.Attachment{
			Type:     t,
			URL:      a.URL,
			FileName: a.FileName,
		})
	}
	return atts
}

// guessAttachmentType 根据文件名或 URL 推断附件类型
func guessAttachmentType(url, fileName string) channels.MessageType {
	name := strings.ToLower(fileName)
	if name == "" {
		name = strings.ToLower(url)
	}
	switch {
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"),
		strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".gif"),
		strings.HasSuffix(name, ".webp"), strings.HasSuffix(name, ".bmp"):
		return channels.MsgImage
	case strings.HasSuffix(name, ".mp4"), strings.HasSuffix(name, ".avi"),
		strings.HasSuffix(name, ".mov"), strings.HasSuffix(name, ".mkv"):
		return channels.MsgVideo
	case strings.HasSuffix(name, ".mp3"), strings.HasSuffix(name, ".wav"),
		strings.HasSuffix(name, ".silk"), strings.HasSuffix(name, ".amr"),
		strings.HasSuffix(name, ".ogg"):
		return channels.MsgAudio
	default:
		return channels.MsgFile
	}
}

// isDuplicate 检查消息是否重复（5 分钟内的重复 msgID）
func (c *Channel) isDuplicate(msgID string) bool {
	c.seenMu.Lock()
	defer c.seenMu.Unlock()
	if _, ok := c.seen[msgID]; ok {
		return true
	}
	c.seen[msgID] = time.Now()
	return false
}

// cleanupSeen 定期清理过期的去重记录和空闲会话状态
func (c *Channel) cleanupSeen(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()

			// 清理去重记录（5 分钟过期）
			c.seenMu.Lock()
			seenCutoff := now.Add(-5 * time.Minute)
			for id, t := range c.seen {
				if t.Before(seenCutoff) {
					delete(c.seen, id)
				}
			}
			c.seenMu.Unlock()

			// 清理空闲会话状态（30 分钟无活动）
			c.statesMu.Lock()
			stateCutoff := now.Add(-30 * time.Minute)
			for id, s := range c.states {
				s.mu.Lock()
				idle := s.lastSeen.Before(stateCutoff)
				s.mu.Unlock()
				if idle {
					delete(c.states, id)
				}
			}
			c.statesMu.Unlock()
		}
	}
}

// getState 获取会话状态，不存在则创建
func (c *Channel) getState(chatID string) *chatState {
	c.statesMu.RLock()
	s, ok := c.states[chatID]
	c.statesMu.RUnlock()
	if ok {
		return s
	}
	c.statesMu.Lock()
	defer c.statesMu.Unlock()
	if s, ok = c.states[chatID]; ok {
		return s
	}
	s = &chatState{}
	c.states[chatID] = s
	return s
}

// updateState 更新会话的最近消息 ID
func (c *Channel) updateState(chatID, msgID string) {
	s := c.getState(chatID)
	s.mu.Lock()
	s.msgID = msgID
	s.lastSeen = time.Now()
	s.mu.Unlock()
	s.msgSeq.Store(0)
}
