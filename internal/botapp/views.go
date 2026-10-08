package botapp

import (
	"fmt"
	"html"
	"strings"
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
	cbSubDevices  = "sub:dev:" // sub:dev:<id>
	cbSubTest     = "sub:t:"   // sub:t:<id>
	cbSubRename   = "sub:ren:" // sub:ren:<id>
	cbSubDelete   = "sub:d:"   // sub:d:<id>   -> confirm delete
	cbSubDeleteDo = "sub:dc:"  // sub:dc:<id>
	cbSubOrigins  = "sub:o:"   // sub:o:<id>   -> origins screen
	cbSubOrigDel  = "sub:og:"  // sub:og:<id>  -> delete origin <id>
	cbSubOrigAdd  = "sub:oa:"  // sub:oa:<id>  -> add origin to subscription <id>
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
		[]api.InlineKeyboardButton{
			inlineButton(i18n.T(lang, "btn.test"), fmt.Sprintf("%s%d", cbSubTest, id)),
			inlineButton(i18n.T(lang, "btn.devices"), fmt.Sprintf("%s%d", cbSubDevices, id)),
		},
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.origins"), fmt.Sprintf("%s%d", cbSubOrigins, id))},
		[]api.InlineKeyboardButton{
			inlineButton(i18n.T(lang, "btn.rename"), fmt.Sprintf("%s%d", cbSubRename, id)),
			inlineButton(i18n.T(lang, "btn.delete"), fmt.Sprintf("%s%d", cbSubDelete, id)),
		},
		[]api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.back"), cbSubs)},
	)
}

// kbOrigins lists the origins of a subscription with a delete button each,
// plus a button to add another origin.
func kbOrigins(sub store.Subscription, lang string) *api.InlineKeyboardMarkup {
	rows := make([][]api.InlineKeyboardButton, 0, len(sub.Origins)+2)
	for i, o := range sub.Origins {
		rows = append(rows, []api.InlineKeyboardButton{
			inlineButton(fmt.Sprintf("🗑 %d. %s", i+1, defaultName(o.URL)), fmt.Sprintf("%s%d", cbSubOrigDel, o.ID)),
		})
	}
	rows = append(rows, []api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.origin_add"), fmt.Sprintf("%s%d", cbSubOrigAdd, sub.ID))})
	rows = append(rows, []api.InlineKeyboardButton{inlineButton(i18n.T(lang, "btn.back"), fmt.Sprintf("%s%d", cbSub, sub.ID))})
	return gogramInlineKeyboard(rows...)
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
		len(sub.Origins),
		renderOrigins(sub.Origins, lang),
		sub.CreatedAt.Format(i18n.T(lang, "time.format")),
	)
}

// renderOriginsScreen builds the message listing the origins of a subscription.
func renderOriginsScreen(sub store.Subscription, lang string) string {
	return i18n.T(lang, "origins.title", html.EscapeString(sub.Name), len(sub.Origins)) +
		"\n\n" + renderOrigins(sub.Origins, lang)
}

// renderOrigins lists origins with their URL, HWID and delivery mode.
func renderOrigins(origins []store.Origin, lang string) string {
	lines := make([]string, 0, len(origins))
	for _, o := range origins {
		lines = append(lines, i18n.T(lang, "origin.line",
			html.EscapeString(o.URL),
			html.EscapeString(o.HWID),
			renderMode(o, lang),
		))
	}
	return strings.Join(lines, "\n\n")
}

// renderMode describes how the HWID is passed to the origin.
func renderMode(o store.Origin, lang string) string {
	if o.HWIDMode == store.HWIDModeQuery {
		return i18n.T(lang, "sub.mode.query", html.EscapeString(o.HWIDParam))
	}
	return i18n.T(lang, "sub.mode.header", html.EscapeString(o.HWIDParam))
}

// subscriptionLink builds the public link served on our domain.
func subscriptionLink(baseURL, token string) string {
	return baseURL + "/s/" + token
}

// formatTime renders a timestamp with the locale's layout.
func formatTime(t time.Time, lang string) string {
	return t.Format(i18n.T(lang, "time.format"))
}

// renderDevices builds the message listing the devices of one subscription.
func renderDevices(name, lang string, devices []store.Device) string {
	if len(devices) == 0 {
		return i18n.T(lang, "devices.empty")
	}
	var b strings.Builder
	b.WriteString(i18n.T(lang, "devices.title", html.EscapeString(name), len(devices)))
	for i, d := range devices {
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "%d. ", i+1)
		if d.HWID != "" {
			b.WriteString(i18n.T(lang, "devices.hwid", html.EscapeString(d.HWID)))
		} else {
			b.WriteString(i18n.T(lang, "devices.anon"))
		}
		if meta := deviceMeta(d); meta != "" {
			b.WriteString("\n" + i18n.T(lang, "devices.device", html.EscapeString(meta)))
		}
		if d.UserAgent != "" {
			b.WriteString("\n" + i18n.T(lang, "devices.ua", html.EscapeString(d.UserAgent)))
		}
		if d.IP != "" {
			b.WriteString("\n" + i18n.T(lang, "devices.ip", html.EscapeString(d.IP)))
		}
		b.WriteString("\n" + i18n.T(lang, "devices.stats", d.Requests, formatTime(d.LastSeen, lang)))
	}
	return b.String()
}

// deviceMeta renders the model and OS of a device, skipping empty parts.
func deviceMeta(d store.Device) string {
	os := strings.TrimSpace(strings.TrimSpace(d.OS) + " " + strings.TrimSpace(d.OSVersion))
	parts := make([]string, 0, 2)
	if d.Model != "" {
		parts = append(parts, d.Model)
	}
	if os != "" {
		parts = append(parts, os)
	}
	return strings.Join(parts, " · ")
}
