package botapp

import (
	"fmt"
	"html"
	"time"

	gogram "github.com/infybtw/GoGramm"
	"github.com/infybtw/GoGramm/api"

	"github.com/infybtw/ProxyConnectorBot/internal/i18n"
	"github.com/infybtw/ProxyConnectorBot/internal/store"
)

// inlineButton and gogramInlineKeyboard are thin aliases of the GoGramm
// keyboard helpers, kept local for brevity of the view builders below.
func inlineButton(text, data string) api.InlineKeyboardButton {
	return gogram.InlineButton(text, data)
}

func gogramInlineKeyboard(rows ...[]api.InlineKeyboardButton) *api.InlineKeyboardMarkup {
	return gogram.InlineKeyboard(rows...)
}

// Callback data prefixes. Telegram limits callback_data to 64 bytes.
const (
	cbMenu        = "menu"
	cbAdd         = "add"
	cbAddNameSkip = "add:name:skip"
	cbSubs        = "subs"
	cbLang        = "lang"
	cbLangPrefix  = "lang:"

	cbSub         = "sub:"     // sub:<id>
	cbSubTest     = "sub:t:"   // sub:t:<id>
	cbSubRename   = "sub:ren:" // sub:ren:<id>
	cbSubDelete   = "sub:d:"   // sub:d:<id>   -> confirm delete
	cbSubDeleteDo = "sub:dc:"  // sub:dc:<id>
)

// kbMenu is the main menu keyboard.
func kbMenu(lang string) *api.InlineKeyboardMarkup {
	return gogramInlineKeyboard(
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.add"), cbAdd)},
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.subs"), cbSubs)},
		[]api.InlineKeyboardButton{
			inlineButton(i18n.T(lang, "btn.lang"), cbLang),
			inlineButton(i18n.T(lang, "btn.help"), "help"),
		},
	)
}

// kbBack returns a keyboard with a single "back to menu" button.
func kbBack(lang string) *api.InlineKeyboardMarkup {
	return gogramInlineKeyboard(
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.back"), cbMenu)},
	)
}

// kbLang is the language selection keyboard.
func kbLang() *api.InlineKeyboardMarkup {
	return gogramInlineKeyboard(
		[]api.InlineKeyboardButton{inlineButton("🇷🇺 Русский", cbLangPrefix+i18n.LangRU)},
		[]api.InlineKeyboardButton{inlineButton("🇬🇧 English", cbLangPrefix+i18n.LangEN)},
		[]api.InlineKeyboardButton{inlineButton("⬅️ Back", cbMenu)},
	)
}

// kbSubs lists subscriptions as buttons.
func kbSubs(subs []store.Subscription, lang string) *api.InlineKeyboardMarkup {
	rows := make([][]api.InlineKeyboardButton, 0, len(subs)+1)
	for _, sub := range subs {
		rows = append(rows, []api.InlineKeyboardButton{
			inlineButton(fmt.Sprintf("▫️ %s", sub.Name), fmt.Sprintf("%s%d", cbSub, sub.ID)),
		})
	}
	rows = append(rows, []api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.add"), cbAdd)})
	rows = append(rows, []api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.back"), cbMenu)})
	return gogramInlineKeyboard(rows...)
}

// kbSub is the detail keyboard of one subscription.
func kbSub(id int64, lang string) *api.InlineKeyboardMarkup {
	return gogramInlineKeyboard(
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.test"), fmt.Sprintf("%s%d", cbSubTest, id))},
		[]api.InlineKeyboardButton{
			inlineButton(i18n.T(lang, "btn.rename"), fmt.Sprintf("%s%d", cbSubRename, id)),
			inlineButton(i18n.T(lang, "btn.delete"), fmt.Sprintf("%s%d", cbSubDelete, id)),
		},
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.back"), cbSubs)},
	)
}

// kbDeleteConfirm asks for a delete confirmation.
func kbDeleteConfirm(id int64, lang string) *api.InlineKeyboardMarkup {
	return gogramInlineKeyboard(
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.delete_confirm"), fmt.Sprintf("%s%d", cbSubDeleteDo, id))},
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.cancel"), fmt.Sprintf("%s%d", cbSub, id))},
	)
}

// kbCancelFlow offers to cancel an interactive flow.
func kbCancelFlow(lang string) *api.InlineKeyboardMarkup {
	return gogramInlineKeyboard(
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.cancel"), "flow:cancel")},
	)
}

// kbAddNameSkip offers to skip the name step of the add flow.
func kbAddNameSkip(lang string) *api.InlineKeyboardMarkup {
	return gogramInlineKeyboard(
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.skip"), cbAddNameSkip)},
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.cancel"), "flow:cancel")},
	)
}

// renderSubDetail builds the detail message of one subscription.
func renderSubDetail(sub store.Subscription, lang, baseURL string) string {
	return i18n.T(lang, "sub.detail",
		html.EscapeString(sub.Name),
		subscriptionLink(baseURL, sub.Token),
		html.EscapeString(sub.OriginURL),
		html.EscapeString(sub.HWID),
		renderMode(sub, lang),
		sub.CreatedAt.Format(i18n.T(lang, "time.format")),
	)
}

// renderMode describes how the HWID is passed to the origin.
func renderMode(sub store.Subscription, lang string) string {
	if sub.HWIDMode == store.HWIDModeQuery {
		return i18n.T(lang, "sub.mode.query", html.EscapeString(sub.HWIDParam))
	}
	return i18n.T(lang, "sub.mode.header", html.EscapeString(sub.HWIDParam))
}

// subscriptionLink builds the public link served on our domain.
func subscriptionLink(baseURL, token string) string {
	return baseURL + "/s/" + token
}

// formatTime renders a timestamp with the locale's layout.
func formatTime(t time.Time, lang string) string {
	return t.Format(i18n.T(lang, "time.format"))
}
