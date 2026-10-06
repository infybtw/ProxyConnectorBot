package botapp

import (
	"context"
	"errors"
	"log/slog"

	gogram "github.com/infybtw/GoGramm"

	"github.com/infybtw/ProxyConnectorBot/internal/i18n"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// onCallback dispatches inline button presses. Callback payloads carry the
// subscription id: sub:<id> and friends.
func (b *Bot) onCallback(c *gogram.Context) error {
	if c.Update == nil || c.Update.CallbackQuery == nil || c.Update.CallbackQuery.Data == nil {
		return nil
	}
	data := *c.Update.CallbackQuery.Data
	lang := b.langOf(c)
	uid := c.Update.CallbackQuery.From.ID

	switch {
	case data == cbMenu:
		answerCallback(c, "", false)
		editHTML(c, i18n.T(lang, "start.menu"), kbMenu(lang))

	case data == "help":
		answerCallback(c, "", false)
		editHTML(c, i18n.T(lang, "help.text", subscriptionLink(b.cfg.PublicBaseURL, "TOKEN")), kbMenu(lang))

	case data == cbSubs:
		answerCallback(c, "", false)
		b.editSubsList(c, uid, lang)

	case data == cbAdd:
		answerCallback(c, "", false)
		b.startFlow(uid, &flow{kind: flowAddURL})
		editHTML(c, i18n.T(lang, "add.ask_url"), kbCancelFlow(lang))

	case data == cbAddNameSkip:
		answerCallback(c, "", false)
		b.completeAddWithoutName(c, uid, lang)

	case data == cbLang:
		answerCallback(c, "", false)
		editHTML(c, i18n.T(lang, "lang.choose"), kbLang())

	case data == cbLangPrefix+i18n.LangRU || data == cbLangPrefix+i18n.LangEN:
		code := data[len(cbLangPrefix):]
		answerCallback(c, "", false)
		if err := b.store.SetLang(context.Background(), uid, code); err == nil {
			editHTML(c, i18n.T(code, "lang.set"), kbMenu(code))
		}

	case data == "flow:cancel":
		answerCallback(c, i18n.T(lang, "flow.canceled"), false)
		b.clearFlow(uid)
		editHTML(c, i18n.T(lang, "start.menu"), kbMenu(lang))

	default:
		b.dispatchSubCallback(c, uid, lang, data)
	}
	return nil
}

// dispatchSubCallback handles subscription-scoped button presses. The most
// specific prefixes are matched first.
func (b *Bot) dispatchSubCallback(c *gogram.Context, uid int64, lang, data string) {
	switch {
	case hasPrefixAndID(cbSubDeleteDo, data):
		id, _ := parseID(cbSubDeleteDo, data)
		b.actDelete(c, uid, lang, id)

	case hasPrefixAndID(cbSubDelete, data):
		id, _ := parseID(cbSubDelete, data)
		sub, ok := b.loadSub(c, uid, lang, id)
		if !ok {
			return
		}
		answerCallback(c, "", false)
		editHTML(c, i18n.T(lang, "sub.delete_confirm", sub.Name), kbDeleteConfirm(id, lang))

	case hasPrefixAndID(cbSubRename, data):
		id, _ := parseID(cbSubRename, data)
		if _, ok := b.loadSub(c, uid, lang, id); !ok {
			return
		}
		answerCallback(c, "", false)
		b.startFlow(uid, &flow{kind: flowRename, subID: id})
		editHTML(c, i18n.T(lang, "sub.rename.ask"), kbCancelFlow(lang))

	case hasPrefixAndID(cbSubTest, data):
		id, _ := parseID(cbSubTest, data)
		b.actTest(c, uid, lang, id)

	case hasPrefixAndID(cbSub, data):
		id, _ := parseID(cbSub, data)
		sub, ok := b.loadSub(c, uid, lang, id)
		if !ok {
			return
		}
		answerCallback(c, "", false)
		editHTML(c, renderSubDetail(sub, lang, b.cfg.PublicBaseURL), kbSub(id, lang))

	default:
		answerCallback(c, "", false)
	}
}

// actTest fetches the subscription from origin with the stored HWID and
// reports the outcome.
func (b *Bot) actTest(c *gogram.Context, uid int64, lang string, id int64) {
	sub, ok := b.loadSub(c, uid, lang, id)
	if !ok {
		return
	}
	answerCallback(c, i18n.T(lang, "btn.test")+"…", false)

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.OriginTimeout)
	defer cancel()
	res, err := b.origin.Fetch(ctx, sub)
	if err != nil {
		sendHTML(c, i18n.T(lang, "sub.test_fail", err.Error()), kbSub(id, lang))
		return
	}
	sendHTML(c, i18n.T(lang, "sub.test_ok", res.StatusCode, len(res.Body), res.Header.Get("Content-Type")), kbSub(id, lang))
}

// actDelete removes a subscription after confirmation.
func (b *Bot) actDelete(c *gogram.Context, uid int64, lang string, id int64) {
	sub, err := b.store.GetSubscription(context.Background(), uid, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			answerCallback(c, "", false)
			editHTML(c, i18n.T(lang, "sub.not_found"), kbBack(lang))
			return
		}
		answerCallback(c, "", false)
		sendHTML(c, i18n.T(lang, "err.db"), nil)
		return
	}
	if err := b.store.DeleteSubscription(context.Background(), uid, id); err != nil {
		answerCallback(c, "", false)
		sendHTML(c, i18n.T(lang, "err.db"), nil)
		return
	}
	answerCallback(c, i18n.T(lang, "sub.deleted", sub.Name), false)
	b.editSubsList(c, uid, lang)
}

// editSubsList renders the subscription list into the current message.
func (b *Bot) editSubsList(c *gogram.Context, uid int64, lang string) {
	subs, err := b.store.ListSubscriptions(context.Background(), uid)
	if err != nil {
		editHTML(c, i18n.T(lang, "err.db"), kbBack(lang))
		return
	}
	if len(subs) == 0 {
		editHTML(c, i18n.T(lang, "subs.empty"), kbMenu(lang))
		return
	}
	editHTML(c, i18n.T(lang, "subs.title", len(subs)), kbSubs(subs, lang))
}

// loadSub fetches a subscription owned by the user, reporting "not found"
// into the current message when it is missing.
func (b *Bot) loadSub(c *gogram.Context, uid int64, lang string, id int64) (store.Subscription, bool) {
	sub, err := b.store.GetSubscription(context.Background(), uid, id)
	if err != nil {
		answerCallback(c, "", false)
		if errors.Is(err, store.ErrNotFound) {
			editHTML(c, i18n.T(lang, "sub.not_found"), kbBack(lang))
		} else {
			slog.Error("bot: load subscription failed", "id", id, "err", err)
			sendHTML(c, i18n.T(lang, "err.db"), nil)
		}
		return store.Subscription{}, false
	}
	return sub, true
}

// hasPrefixAndID reports whether data is "prefix<numeric id>".
func hasPrefixAndID(prefix, data string) bool {
	_, ok := parseID(prefix, data)
	return ok
}
