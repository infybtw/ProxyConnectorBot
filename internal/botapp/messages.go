package botapp

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"

	gogram "github.com/infybtw/GoGramm"

	"github.com/infybtw/ProxyConnectorBot/internal/hwid"
	"github.com/infybtw/ProxyConnectorBot/internal/i18n"
	"github.com/infybtw/ProxyConnectorBot/internal/origin"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// cmdStart greets the user and shows the main menu.
func (b *Bot) cmdStart(c *gogram.Context) error {
	lang := b.langOf(c)
	sendHTML(c, i18n.T(lang, "start.welcome")+"\n\n"+i18n.T(lang, "start.menu"), kbMenu(lang))
	return nil
}

// cmdHelp explains the workflow and lists commands.
func (b *Bot) cmdHelp(c *gogram.Context) error {
	lang := b.langOf(c)
	example := subscriptionLink(b.cfg.PublicBaseURL, "TOKEN")
	sendHTML(c, i18n.T(lang, "help.text", example), kbMenu(lang))
	return nil
}

// cmdLang shows the language picker.
func (b *Bot) cmdLang(c *gogram.Context) error {
	lang := b.langOf(c)
	sendHTML(c, i18n.T(lang, "lang.choose"), kbLang())
	return nil
}

// cmdAdd starts the add-subscription flow.
func (b *Bot) cmdAdd(c *gogram.Context) error {
	lang := b.langOf(c)
	id, _ := senderID(c)
	b.startFlow(id, &flow{kind: flowAddURL})
	sendHTML(c, i18n.T(lang, "add.ask_url"), kbCancelFlow(lang))
	return nil
}

// cmdSubs lists the subscriptions of the user.
func (b *Bot) cmdSubs(c *gogram.Context) error {
	lang := b.langOf(c)
	uid, _ := senderID(c)
	subs, err := b.store.ListSubscriptions(context.Background(), uid)
	if err != nil {
		sendHTML(c, i18n.T(lang, "err.db"), kbMenu(lang))
		return err
	}
	if len(subs) == 0 {
		sendHTML(c, i18n.T(lang, "subs.empty"), kbMenu(lang))
		return nil
	}
	sendHTML(c, i18n.T(lang, "subs.title", len(subs)), kbSubs(subs, lang))
	return nil
}

// cmdCancel drops the active flow.
func (b *Bot) cmdCancel(c *gogram.Context) error {
	lang := b.langOf(c)
	id, _ := senderID(c)
	b.clearFlow(id)
	sendHTML(c, i18n.T(lang, "flow.canceled"), kbMenu(lang))
	return nil
}

// onMessage routes free-text messages: active flows consume input first.
func (b *Bot) onMessage(c *gogram.Context) error {
	lang := b.langOf(c)
	id, ok := senderID(c)
	if !ok {
		return nil
	}
	text := messageText(c)
	if text == "" {
		return nil
	}

	f := b.peekFlow(id)
	if f == nil {
		// Ignore unknown slash commands, nudge everything else.
		if !strings.HasPrefix(text, "/") {
			sendHTML(c, i18n.T(lang, "flow.unknown"), kbMenu(lang))
		}
		return nil
	}

	if isCancelText(text) {
		b.clearFlow(id)
		sendHTML(c, i18n.T(lang, "flow.canceled"), kbMenu(lang))
		return nil
	}

	switch f.kind {
	case flowAddURL:
		return b.flowAddURL(c, id, lang, text)
	case flowAddName:
		return b.flowAddName(c, id, lang, text)
	case flowRename:
		return b.flowRename(c, id, lang, text)
	default:
		b.clearFlow(id)
		sendHTML(c, i18n.T(lang, "flow.unknown"), kbMenu(lang))
		return nil
	}
}

// flowAddURL validates the origin URL and asks for a display name.
func (b *Bot) flowAddURL(c *gogram.Context, uid int64, lang, text string) error {
	rawURL := strings.TrimSpace(text)
	if !origin.IsHTTPURL(rawURL) {
		sendHTML(c, i18n.T(lang, "add.invalid_url"), kbCancelFlow(lang))
		return nil
	}
	mode, param := detectHWIDMode(rawURL)
	b.startFlow(uid, &flow{
		kind:      flowAddName,
		url:       rawURL,
		hwidMode:  mode,
		hwidParam: param,
	})
	sendHTML(c, i18n.T(lang, "add.ask_name"), kbAddNameSkip(lang))
	return nil
}

// flowAddName finalizes the add-subscription flow.
func (b *Bot) flowAddName(c *gogram.Context, uid int64, lang, text string) error {
	f := b.takeFlow(uid)
	if f == nil {
		return nil
	}
	name := strings.TrimSpace(text)
	if len([]rune(name)) == 0 || len([]rune(name)) > 64 {
		b.startFlow(uid, f)
		sendHTML(c, i18n.T(lang, "add.invalid_name"), kbAddNameSkip(lang))
		return nil
	}
	b.createSubscription(c, uid, lang, f, name)
	return nil
}

// completeAddWithoutName finishes the add flow with a name derived from the
// origin host.
func (b *Bot) completeAddWithoutName(c *gogram.Context, uid int64, lang string) {
	f := b.takeFlow(uid)
	if f == nil {
		return
	}
	if f.kind != flowAddName {
		// The URL step is not finished yet: keep the flow and re-ask.
		b.startFlow(uid, f)
		sendHTML(c, i18n.T(lang, "add.ask_url"), kbCancelFlow(lang))
		return
	}
	b.createSubscription(c, uid, lang, f, defaultName(f.url))
}

// createSubscription generates the HWID once and stores the subscription.
func (b *Bot) createSubscription(c *gogram.Context, uid int64, lang string, f *flow, name string) {
	generated, err := hwid.Generate()
	if err != nil {
		slog.Error("bot: hwid generation failed", "err", err)
		sendHTML(c, i18n.T(lang, "err.generic"), kbMenu(lang))
		return
	}
	sub := &store.Subscription{
		UserID:    uid,
		Name:      name,
		OriginURL: f.url,
		HWID:      generated,
		HWIDMode:  f.hwidMode,
		HWIDParam: f.hwidParam,
	}
	if err := b.store.CreateSubscription(context.Background(), sub); err != nil {
		slog.Error("bot: create subscription failed", "err", err)
		sendHTML(c, i18n.T(lang, "err.db"), kbMenu(lang))
		return
	}
	sendHTML(c, i18n.T(lang, "add.created",
		name,
		subscriptionLink(b.cfg.PublicBaseURL, sub.Token),
		sub.HWID,
		renderMode(*sub, lang),
	), kbSub(sub.ID, lang))
}

// flowRename saves a new subscription name.
func (b *Bot) flowRename(c *gogram.Context, uid int64, lang, text string) error {
	f := b.takeFlow(uid)
	if f == nil {
		return nil
	}
	name := strings.TrimSpace(text)
	if len([]rune(name)) == 0 || len([]rune(name)) > 64 {
		b.startFlow(uid, f)
		sendHTML(c, i18n.T(lang, "add.invalid_name"), kbCancelFlow(lang))
		return nil
	}
	if err := b.store.RenameSubscription(context.Background(), uid, f.subID, name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			sendHTML(c, i18n.T(lang, "sub.not_found"), kbMenu(lang))
			return nil
		}
		sendHTML(c, i18n.T(lang, "err.db"), kbMenu(lang))
		return err
	}
	sendHTML(c, i18n.T(lang, "sub.renamed", name)+b.detailBlock(uid, f.subID, lang), kbSub(f.subID, lang))
	return nil
}

// detailBlock renders the detail card of a subscription as an appendix.
func (b *Bot) detailBlock(uid, subID int64, lang string) string {
	sub, err := b.store.GetSubscription(context.Background(), uid, subID)
	if err != nil {
		return ""
	}
	return "\n\n" + renderSubDetail(sub, lang, b.cfg.PublicBaseURL)
}

// detectHWIDMode guesses how the origin expects the HWID: if the URL already
// carries a hwid-like query parameter we mirror it as a query parameter,
// otherwise we use the x-hwid header (Happ/INCY style).
func detectHWIDMode(rawURL string) (mode, param string) {
	u, err := url.Parse(rawURL)
	if err == nil {
		for key := range u.Query() {
			if strings.Contains(strings.ToLower(key), "hwid") {
				return store.HWIDModeQuery, key
			}
		}
	}
	return store.HWIDModeHeader, "x-hwid"
}

// defaultName derives a display name from the origin host.
func defaultName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "subscription"
	}
	return u.Host
}

// isCancelText matches localized cancel commands typed as plain text.
func isCancelText(text string) bool {
	switch strings.ToLower(text) {
	case "/cancel", "cancel", "отмена", "✖️ отмена", "✖️ cancel":
		return true
	}
	return false
}
