package weixin

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	channel "fkteams/internal/adapters/transport/channel"
	wechatbot "fkteams/internal/adapters/transport/channel/weixin/sdk"
	"fkteams/internal/runtime/log"

	qrcode "github.com/skip2/go-qrcode"
)

// Register 注册微信通道工厂。
func Register(registry *channel.FactoryRegistry) {
	registry.Register("weixin", NewChannel)
}

// Channel 微信机器人通道
type Channel struct {
	baseURL   string
	credPath  string
	logLevel  string
	allowFrom map[string]bool

	bot       *wechatbot.Bot
	handler   channel.MessageHandler
	running   atomic.Bool
	cancel    context.CancelFunc
	runCtx    context.Context
	runDone   <-chan error
	accepting bool
	mu        sync.Mutex

	typingMu      sync.Mutex
	typingCancels map[string]typingIndicator
	typingSeq     uint64
}

const (
	maxTypingIndicators = 256
	typingRefresh       = 8 * time.Second
	typingMaxDuration   = 2 * time.Minute
)

type typingIndicator struct {
	id        uint64
	cancel    context.CancelFunc
	startedAt time.Time
}

// NewChannel 创建微信通道实例
func NewChannel(cfg channel.ChannelConfig, handler channel.MessageHandler) (channel.Channel, error) {
	c := &Channel{
		baseURL:       cfg.Extra["base_url"],
		credPath:      cfg.Extra["cred_path"],
		logLevel:      cfg.Extra["log_level"],
		handler:       handler,
		typingCancels: make(map[string]typingIndicator),
	}
	if ids := cfg.Extra["allow_from"]; ids != "" {
		c.allowFrom = make(map[string]bool)
		for _, id := range strings.Split(ids, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				c.allowFrom[id] = true
			}
		}
	}
	return c, nil
}

func (c *Channel) Name() string    { return "weixin" }
func (c *Channel) IsRunning() bool { return c.running.Load() }

// Start 启动微信 Bot，执行登录并开始消息轮询
func (c *Channel) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !c.running.CompareAndSwap(false, true) {
		return fmt.Errorf("weixin channel is already running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	c.mu.Lock()
	c.cancel = cancel
	c.runCtx = runCtx
	c.runDone = done
	c.accepting = false
	c.mu.Unlock()
	started := false
	defer func() {
		if !started {
			cancel()
			close(done)
			c.mu.Lock()
			if c.runDone == done {
				c.cancel = nil
				c.runCtx = nil
				c.runDone = nil
				c.accepting = false
			}
			c.mu.Unlock()
			c.running.Store(false)
		}
	}()

	opts := wechatbot.Options{
		LogLevel: c.logLevel,
		OnQRURL: func(qrURL string) {
			fmt.Printf("[weixin] 请使用微信扫描以下二维码登录:\n")
			qr, err := qrcode.New(qrURL, qrcode.Medium)
			if err != nil {
				log.Printf("[weixin] 生成二维码失败，请手动打开链接: %s", qrURL)
				return
			}
			fmt.Fprintln(os.Stderr, qr.ToSmallString(false))
		},
		OnScanned: func() {
			fmt.Printf("[weixin] 二维码已扫描，等待确认...\n")
		},
		OnExpired: func() {
			fmt.Printf("[weixin] 二维码已过期\n")
		},
		OnError: func(err error) {
			log.Printf("[weixin] error: %v", err)
		},
	}
	if c.baseURL != "" {
		opts.BaseURL = c.baseURL
	}
	if c.credPath != "" {
		opts.CredPath = c.credPath
	}

	bot := wechatbot.New(opts)

	creds, err := bot.Login(runCtx, false)
	if err != nil {
		return err
	}
	log.Printf("[weixin] 登录成功 (user=%s)", creds.UserID)

	bot.OnMessage(func(msg *wechatbot.IncomingMessage) {
		c.onMessage(bot, msg)
	})

	c.mu.Lock()
	if err := runCtx.Err(); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("start weixin channel: %w", err)
	}
	c.bot = bot
	c.accepting = true
	c.mu.Unlock()
	started = true

	go func() {
		err := bot.Run(runCtx)
		if err != nil && runCtx.Err() == nil {
			log.Printf("[weixin] bot run error: %v", err)
		}
		cancel()
		c.cancelAllTyping()
		c.mu.Lock()
		if c.runDone == done {
			c.bot = nil
			c.runCtx = nil
			c.accepting = false
		}
		c.mu.Unlock()
		done <- err
		close(done)
		c.running.Store(false)
	}()

	return nil
}

// Stop 停止微信 Bot
func (c *Channel) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	bot := c.bot
	cancel := c.cancel
	done := c.runDone
	c.bot = nil
	c.accepting = false
	if cancel != nil {
		cancel()
	}
	c.mu.Unlock()
	c.cancelAllTyping()
	if bot != nil {
		bot.Stop()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return fmt.Errorf("stop weixin channel: %w", ctx.Err())
		}
	}
	c.mu.Lock()
	if c.runDone == done {
		c.bot = nil
		c.cancel = nil
		c.runCtx = nil
		c.runDone = nil
	}
	c.mu.Unlock()
	c.running.Store(false)
	log.Printf("[weixin] 微信 bot 已停止")
	return nil
}

// Send 向指定用户发送消息
func (c *Channel) Send(ctx context.Context, chatID string, msg channel.Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	bot := c.bot
	accepting := c.accepting
	c.mu.Unlock()
	if bot == nil || !accepting {
		return fmt.Errorf("weixin channel is not running")
	}

	// 发送消息前停止 typing 指示器
	c.stopTyping(chatID)

	if msg.Content != "" {
		if err := bot.Send(ctx, chatID, msg.Content); err != nil {
			return err
		}
	}

	return nil
}

// onMessage 处理收到的微信消息
func (c *Channel) onMessage(bot *wechatbot.Bot, msg *wechatbot.IncomingMessage) {
	if len(c.allowFrom) > 0 && !c.allowFrom[msg.UserID] {
		return
	}

	content := strings.TrimSpace(msg.Text)
	var attachments []channel.Attachment

	for _, img := range msg.Images {
		a := channel.Attachment{Type: channel.MsgImage}
		if img.URL != "" {
			a.URL = img.URL
		}
		attachments = append(attachments, a)
	}
	for _, v := range msg.Voices {
		a := channel.Attachment{Type: channel.MsgAudio}
		_ = v
		attachments = append(attachments, a)
	}
	for _, vid := range msg.Videos {
		a := channel.Attachment{Type: channel.MsgVideo}
		_ = vid
		attachments = append(attachments, a)
	}
	for _, f := range msg.Files {
		a := channel.Attachment{Type: channel.MsgFile, FileName: f.FileName}
		attachments = append(attachments, a)
	}

	if content == "" && len(attachments) == 0 {
		return
	}

	inMsg := channel.Message{Content: content, Attachments: attachments}
	if len(attachments) > 0 {
		inMsg.Type = attachments[0].Type
	}

	msgCtx := channel.WithChannelName(context.Background(), "weixin")
	// 启动 typing 指示器（持续刷新，直到 Send 时停止）
	c.startTyping(bot, msg.UserID)
	c.handler(msgCtx, msg.UserID, msg.UserID, inMsg, false)
}

// startTyping 启动持续 typing 指示器
func (c *Channel) startTyping(bot *wechatbot.Bot, userID string) {
	if bot == nil || userID == "" {
		return
	}
	c.mu.Lock()
	parent := c.runCtx
	active := c.bot == bot
	c.mu.Unlock()
	if parent == nil || !active {
		return
	}
	ctx, cancel := context.WithTimeout(parent, typingMaxDuration)
	id := c.registerTyping(userID, cancel)

	go func() {
		defer cancel()
		defer c.removeTyping(userID, id)
		if err := bot.SendTyping(ctx, userID); err != nil {
			return
		}
		ticker := time.NewTicker(typingRefresh)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := bot.SendTyping(ctx, userID); err != nil {
					return
				}
			}
		}
	}()
}

func (c *Channel) registerTyping(userID string, cancel context.CancelFunc) uint64 {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	if previous, ok := c.typingCancels[userID]; ok {
		previous.cancel()
		delete(c.typingCancels, userID)
	}
	for len(c.typingCancels) >= maxTypingIndicators {
		var oldestUser string
		var oldestTime time.Time
		for candidate, indicator := range c.typingCancels {
			if oldestUser == "" || indicator.startedAt.Before(oldestTime) {
				oldestUser = candidate
				oldestTime = indicator.startedAt
			}
		}
		oldest := c.typingCancels[oldestUser]
		oldest.cancel()
		delete(c.typingCancels, oldestUser)
	}
	c.typingSeq++
	id := c.typingSeq
	c.typingCancels[userID] = typingIndicator{id: id, cancel: cancel, startedAt: time.Now()}
	return id
}

// stopTyping 停止指定用户的 typing 指示器
func (c *Channel) stopTyping(userID string) {
	c.typingMu.Lock()
	if indicator, ok := c.typingCancels[userID]; ok {
		indicator.cancel()
		delete(c.typingCancels, userID)
	}
	c.typingMu.Unlock()
}

func (c *Channel) removeTyping(userID string, id uint64) {
	c.typingMu.Lock()
	if indicator, ok := c.typingCancels[userID]; ok && indicator.id == id {
		delete(c.typingCancels, userID)
	}
	c.typingMu.Unlock()
}

func (c *Channel) cancelAllTyping() {
	c.typingMu.Lock()
	for userID, indicator := range c.typingCancels {
		indicator.cancel()
		delete(c.typingCancels, userID)
	}
	c.typingMu.Unlock()
}
