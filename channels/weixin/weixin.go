package weixin

import (
	"context"
	"fkteams/channels"
	wechatbot "fkteams/channels/weixin/sdk"
	"fkteams/log"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

func init() {
	channels.RegisterFactory("weixin", NewChannel)
}

// Channel 微信机器人通道
type Channel struct {
	baseURL   string
	credPath  string
	logLevel  string
	allowFrom map[string]bool

	bot     *wechatbot.Bot
	handler channels.MessageHandler
	running atomic.Bool
	cancel  context.CancelFunc
	mu      sync.Mutex

	typingMu      sync.Mutex
	typingCancels map[string]context.CancelFunc // per-user typing cancel
}

// NewChannel 创建微信通道实例
func NewChannel(cfg channels.ChannelConfig, handler channels.MessageHandler) (channels.Channel, error) {
	c := &Channel{
		baseURL:       cfg.Extra["base_url"],
		credPath:      cfg.Extra["cred_path"],
		logLevel:      cfg.Extra["log_level"],
		handler:       handler,
		typingCancels: make(map[string]context.CancelFunc),
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

	creds, err := bot.Login(ctx, false)
	if err != nil {
		return err
	}
	log.Printf("[weixin] 登录成功 (user=%s)", creds.UserID)

	bot.OnMessage(func(msg *wechatbot.IncomingMessage) {
		c.onMessage(bot, msg)
	})

	c.mu.Lock()
	c.bot = bot
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()

	c.running.Store(true)

	go func() {
		if err := bot.Run(runCtx); err != nil {
			log.Printf("[weixin] bot run error: %v", err)
		}
		c.running.Store(false)
	}()

	return nil
}

// Stop 停止微信 Bot
func (c *Channel) Stop(_ context.Context) error {
	c.mu.Lock()
	bot := c.bot
	cancel := c.cancel
	c.bot = nil
	c.cancel = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if bot != nil {
		bot.Stop()
	}
	c.running.Store(false)
	log.Printf("[weixin] 微信 bot 已停止")
	return nil
}

// Send 向指定用户发送消息
func (c *Channel) Send(ctx context.Context, chatID string, msg channels.Message) error {
	c.mu.Lock()
	bot := c.bot
	c.mu.Unlock()
	if bot == nil {
		return nil
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
	var attachments []channels.Attachment

	for _, img := range msg.Images {
		a := channels.Attachment{Type: channels.MsgImage}
		if img.URL != "" {
			a.URL = img.URL
		}
		attachments = append(attachments, a)
	}
	for _, v := range msg.Voices {
		a := channels.Attachment{Type: channels.MsgAudio}
		_ = v
		attachments = append(attachments, a)
	}
	for _, vid := range msg.Videos {
		a := channels.Attachment{Type: channels.MsgVideo}
		_ = vid
		attachments = append(attachments, a)
	}
	for _, f := range msg.Files {
		a := channels.Attachment{Type: channels.MsgFile, FileName: f.FileName}
		attachments = append(attachments, a)
	}

	if content == "" && len(attachments) == 0 {
		return
	}

	inMsg := channels.Message{Content: content, Attachments: attachments}
	if len(attachments) > 0 {
		inMsg.Type = attachments[0].Type
	}

	msgCtx := channels.WithChannelName(context.Background(), "weixin")
	// 启动 typing 指示器（持续刷新，直到 Send 时停止）
	c.startTyping(bot, msg.UserID)
	c.handler(msgCtx, msg.UserID, msg.UserID, inMsg, false)
}

// startTyping 启动持续 typing 指示器
func (c *Channel) startTyping(bot *wechatbot.Bot, userID string) {
	c.stopTyping(userID) // 先停止之前的

	ctx, cancel := context.WithCancel(context.Background())
	c.typingMu.Lock()
	c.typingCancels[userID] = cancel
	c.typingMu.Unlock()

	go func() {
		_ = bot.SendTyping(ctx, userID)
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = bot.SendTyping(ctx, userID)
			}
		}
	}()
}

// stopTyping 停止指定用户的 typing 指示器
func (c *Channel) stopTyping(userID string) {
	c.typingMu.Lock()
	if cancel, ok := c.typingCancels[userID]; ok {
		cancel()
		delete(c.typingCancels, userID)
	}
	c.typingMu.Unlock()
}
