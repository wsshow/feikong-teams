package discord

import (
	"context"
	"fkteams/channels"
	"fkteams/fkenv"
	"fkteams/log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
)

func init() {
	channels.RegisterFactory("discord", NewChannel)
}

// Channel Discord 机器人通道
type Channel struct {
	token     string
	allowFrom map[string]bool

	session *discordgo.Session
	handler channels.MessageHandler
	running atomic.Bool
	botID   string
	mu      sync.Mutex
}

// NewChannel 创建 Discord 通道实例
func NewChannel(cfg channels.ChannelConfig, handler channels.MessageHandler) (channels.Channel, error) {
	c := &Channel{
		token:   cfg.Extra["token"],
		handler: handler,
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

func (c *Channel) Name() string    { return "discord" }
func (c *Channel) IsRunning() bool { return c.running.Load() }

// Start 启动 Discord Bot WebSocket 连接
func (c *Channel) Start(ctx context.Context) error {
	session, err := discordgo.New("Bot " + strings.TrimPrefix(c.token, "Bot "))
	if err != nil {
		return err
	}

	// 配置代理（FEIKONG_PROXY_URL）
	if proxyStr := fkenv.Get(fkenv.ProxyURL); proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err == nil {
			session.Client = &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyURL),
				},
			}
			session.Dialer = &websocket.Dialer{
				Proxy: http.ProxyURL(proxyURL),
			}
			log.Printf("[discord] using proxy: %s", proxyStr)
		}
	}

	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	session.AddHandler(c.messageCreate)

	if err := session.Open(); err != nil {
		return err
	}

	c.mu.Lock()
	c.session = session
	c.botID = session.State.User.ID
	c.mu.Unlock()

	c.running.Store(true)
	log.Printf("[discord] Discord bot started (user=%s)", session.State.User.Username)

	go func() {
		<-ctx.Done()
		c.Stop(context.Background())
	}()

	return nil
}

// Stop 停止 Discord Bot
func (c *Channel) Stop(_ context.Context) error {
	c.mu.Lock()
	session := c.session
	c.session = nil
	c.mu.Unlock()

	if session != nil {
		_ = session.Close()
	}
	c.running.Store(false)
	log.Printf("[discord] Discord bot stopped")
	return nil
}

// Send 向指定频道发送消息
func (c *Channel) Send(_ context.Context, chatID string, msg channels.Message) error {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == nil {
		return nil
	}

	channelID := extractChannelID(chatID)

	if msg.Content != "" {
		_, err := session.ChannelMessageSend(channelID, msg.Content)
		if err != nil {
			return err
		}
	}

	return nil
}

// messageCreate 处理收到的消息
func (c *Channel) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == c.botID {
		return
	}
	if m.Author.Bot {
		return
	}

	if len(c.allowFrom) > 0 && !c.allowFrom[m.Author.ID] {
		return
	}

	isDM := isDMChannel(s, m)
	isGroup := !isDM

	if isGroup {
		mentioned := false
		for _, u := range m.Mentions {
			if u.ID == c.botID {
				mentioned = true
				break
			}
		}
		if !mentioned {
			return
		}
	}

	var chatID string
	if isDM {
		chatID = "dm:" + m.ChannelID
	} else {
		chatID = "guild:" + m.ChannelID
	}

	content := cleanMentions(m.Content, c.botID)
	content = strings.TrimSpace(content)

	attachments := extractAttachments(m.Attachments)

	if content == "" && len(attachments) == 0 {
		return
	}

	inMsg := channels.Message{Content: content, Attachments: attachments}
	if len(attachments) > 0 {
		inMsg.Type = attachments[0].Type
	}

	ctx := channels.WithChannelName(context.Background(), "discord")
	go func() {
		// 启动 typing 指示器，每 8 秒刷新一次直到处理完成
		typingCtx, cancelTyping := context.WithCancel(ctx)
		defer cancelTyping()
		go c.keepTyping(typingCtx, m.ChannelID)

		c.handler(typingCtx, chatID, m.Author.ID, inMsg, isGroup)
	}()
}

// keepTyping 持续发送 typing 状态直到 ctx 取消
func (c *Channel) keepTyping(ctx context.Context, channelID string) {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == nil {
		return
	}

	_ = session.ChannelTyping(channelID)
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = session.ChannelTyping(channelID)
		}
	}
}

// isDMChannel 判断消息是否来自私聊
func isDMChannel(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	ch, err := s.State.Channel(m.ChannelID)
	if err != nil {
		ch, err = s.Channel(m.ChannelID)
		if err != nil {
			return false
		}
	}
	return ch.Type == discordgo.ChannelTypeDM
}

// cleanMentions 移除文本中对 bot 的 @mention
func cleanMentions(content, botID string) string {
	content = strings.ReplaceAll(content, "<@"+botID+">", "")
	content = strings.ReplaceAll(content, "<@!"+botID+">", "")
	return content
}

// extractAttachments 从 Discord 消息附件中提取 Attachment 列表
func extractAttachments(atts []*discordgo.MessageAttachment) []channels.Attachment {
	if len(atts) == 0 {
		return nil
	}
	var result []channels.Attachment
	for _, a := range atts {
		t := guessAttachmentType(a.Filename, a.ContentType)
		result = append(result, channels.Attachment{
			Type:     t,
			URL:      a.URL,
			FileName: a.Filename,
		})
	}
	return result
}

// guessAttachmentType 根据文件名和 Content-Type 推断附件类型
func guessAttachmentType(fileName, contentType string) channels.MessageType {
	ct := strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(ct, "image/"):
		return channels.MsgImage
	case strings.HasPrefix(ct, "video/"):
		return channels.MsgVideo
	case strings.HasPrefix(ct, "audio/"):
		return channels.MsgAudio
	}
	name := strings.ToLower(fileName)
	switch {
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"),
		strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".gif"),
		strings.HasSuffix(name, ".webp"):
		return channels.MsgImage
	case strings.HasSuffix(name, ".mp4"), strings.HasSuffix(name, ".avi"),
		strings.HasSuffix(name, ".mov"), strings.HasSuffix(name, ".mkv"):
		return channels.MsgVideo
	case strings.HasSuffix(name, ".mp3"), strings.HasSuffix(name, ".wav"),
		strings.HasSuffix(name, ".ogg"), strings.HasSuffix(name, ".flac"):
		return channels.MsgAudio
	default:
		return channels.MsgFile
	}
}

// extractChannelID 从 chatID 中提取 Discord 频道 ID
func extractChannelID(chatID string) string {
	if strings.HasPrefix(chatID, "dm:") {
		return strings.TrimPrefix(chatID, "dm:")
	}
	return strings.TrimPrefix(chatID, "guild:")
}
