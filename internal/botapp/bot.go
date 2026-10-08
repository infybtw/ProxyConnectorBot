// Package botapp implements the Telegram bot interface of the service.
package botapp

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	gogram "github.com/infybtw/GoGramm"
	"github.com/infybtw/GoGramm/api"

	"github.com/infybtw/ProxyConnectorBot/internal/config"
	"github.com/infybtw/ProxyConnectorBot/internal/i18n"
	"github.com/infybtw/ProxyConnectorBot/internal/origin"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// Interactive flow kinds (in-memory, per user).
const (
	flowAddURL    = "add_url"
	flowAddName   = "add_name"
	flowRename    = "rename"
	flowAddOrigin = "add_origin"
)

// flow is the state of an interactive multi-step action.
type flow struct {
	kind      string
	subID     int64
	url       string
	hwidMode  string
	hwidParam string
}

// Bot is the Telegram bot backed by the store and the origin client.
type Bot struct {
	tg     *gogram.Bot
	store  *store.Store
	origin *origin.Client
	cfg    *config.Config

	// defaultLang is the interface language assigned to new users.
	defaultLang string

	mu    sync.Mutex
	flows map[int64]*flow
}

// New creates the bot and registers all handlers.
func New(st *store.Store, oc *origin.Client, cfg *config.Config) *Bot {
	b := &Bot{
		tg:          gogram.NewBot(cfg.TelegramBotToken),
		store:       st,
		origin:      oc,
		cfg:         cfg,
		defaultLang: i18n.Normalize(cfg.DefaultLocale),
		flows:       make(map[int64]*flow),
	}

	b.tg.Command("start", wrap(b.cmdStart))
	b.tg.Command("help", wrap(b.cmdHelp))
	b.tg.Command("lang", wrap(b.cmdLang))
	b.tg.Command("add", wrap(b.cmdAdd))
	b.tg.Command("subs", wrap(b.cmdSubs))
	b.tg.Command("cancel", wrap(b.cmdCancel))

	b.tg.On(gogram.UpdateCallbackQuery, wrap(b.onCallback))
	b.tg.OnMessage(wrap(b.onMessage))

	return b
}

// Run starts long polling (blocking until ctx is cancelled or polling fails).
func (b *Bot) Run(ctx context.Context) error {
	return b.tg.Start(ctx)
}

// wrap keeps a handler error or panic from stopping the polling loop.
func wrap(h func(*gogram.Context) error) gogram.Handler {
	return func(c *gogram.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("bot: handler panic", "panic", r)
			}
			err = nil
		}()
		if err := h(c); err != nil {
			slog.Error("bot: handler error", "err", err)
		}
		return nil
	}
}

// senderID returns the Telegram user id of the update.
func senderID(c *gogram.Context) (int64, bool) {
	if c.Update == nil {
		return 0, false
	}
	switch {
	case c.Update.Message != nil && c.Update.Message.From != nil:
		return c.Update.Message.From.ID, true
	case c.Update.CallbackQuery != nil:
		return c.Update.CallbackQuery.From.ID, true
	}
	return 0, false
}

// user loads the sender's profile, creating it on first contact. New users
// start in the configured DEFAULT_LOCALE; language changes made via /lang are
// persisted and take precedence from then on.
func (b *Bot) user(c *gogram.Context) (store.User, error) {
	id, ok := senderID(c)
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	if err := b.store.UpsertUser(context.Background(), id, b.defaultLang); err != nil {
		return store.User{}, err
	}
	return b.store.GetUser(context.Background(), id)
}

// langOf is the interface language of the sender.
func (b *Bot) langOf(c *gogram.Context) string {
	u, err := b.user(c)
	if err != nil {
		return b.defaultLang
	}
	return u.Lang
}

// answerCallback acknowledges a callback query; when text is not empty it is
// shown as a toast (or alert when showAlert is set).
func answerCallback(c *gogram.Context, text string, showAlert bool) {
	q := c.Update.CallbackQuery
	if q == nil {
		return
	}
	p := &api.AnswerCallbackQueryParams{CallbackQueryID: q.ID}
	if text != "" {
		p.Text = &text
	}
	if showAlert {
		p.ShowAlert = &showAlert
	}
	if err := c.Api.AnswerCallbackQuery(context.Background(), p); err != nil {
		slog.Warn("bot: answerCallbackQuery failed", "err", err)
	}
}

// sendHTML replies with HTML-formatted text and an optional keyboard.
func sendHTML(c *gogram.Context, text string, kb *api.InlineKeyboardMarkup) {
	mode := api.ParseModeHTML
	params := &api.SendMessageParams{ParseMode: &mode}
	if kb != nil {
		params.ReplyMarkup = kb
	}
	if err := c.Reply(text, params); err != nil {
		slog.Warn("bot: reply failed", "err", err)
	}
}

// editHTML replaces the current message text and keyboard.
func editHTML(c *gogram.Context, text string, kb *api.InlineKeyboardMarkup) {
	mode := api.ParseModeHTML
	params := &api.EditMessageTextParams{ParseMode: &mode}
	if kb != nil {
		params.ReplyMarkup = kb
	}
	if err := c.EditMessageText(text, params); err != nil {
		// "message is not modified" and friends are not fatal.
		slog.Debug("bot: edit failed", "err", err)
	}
}

// messageText returns the trimmed text of the incoming message.
func messageText(c *gogram.Context) string {
	if c.Update == nil || c.Update.Message == nil || c.Update.Message.Text == nil {
		return ""
	}
	return strings.TrimSpace(*c.Update.Message.Text)
}

// startFlow begins an interactive flow for the sender.
func (b *Bot) startFlow(userID int64, f *flow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flows[userID] = f
}

// takeFlow returns and clears the active flow of the sender.
func (b *Bot) takeFlow(userID int64) *flow {
	b.mu.Lock()
	defer b.mu.Unlock()
	f := b.flows[userID]
	delete(b.flows, userID)
	return f
}

// peekFlow returns the active flow without clearing it.
func (b *Bot) peekFlow(userID int64) *flow {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flows[userID]
}

// clearFlow drops the active flow of the sender.
func (b *Bot) clearFlow(userID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.flows, userID)
}

// parseID extracts a numeric id from a "prefix<id>" callback payload.
func parseID(prefix, data string) (int64, bool) {
	if !strings.HasPrefix(data, prefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(data, prefix), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
