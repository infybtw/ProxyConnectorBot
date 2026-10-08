// Keyboard and message builders for the Telegram bot.

import { InlineKeyboard } from "grammy";
import { formatTime, t } from "../i18n";
import { HWID_MODE_QUERY, type Device, type Origin, type Subscription } from "../store";
import { defaultName } from "../url";

/** Callback data prefixes. Telegram limits callback_data to 64 bytes. */
export const CB = {
  menu: "menu",
  add: "add",
  addNameSkip: "add:name:skip",
  subs: "subs",
  lang: "lang",
  langPrefix: "lang:",
  sub: "sub:", // sub:<id>
  subDevices: "sub:dev:", // sub:dev:<id>
  subTest: "sub:t:", // sub:t:<id>
  subRename: "sub:ren:", // sub:ren:<id>
  subDelete: "sub:d:", // sub:d:<id>   -> confirm delete
  subDeleteDo: "sub:dc:", // sub:dc:<id>
  subOrigins: "sub:o:", // sub:o:<id>   -> origins screen
  subOriginSettings: "sub:or:", // sub:or:<id>  -> origin settings <origin id>
  subOriginToggle: "sub:oe:", // sub:oe:<id>  -> enable/disable origin <origin id>
  subOrigDelete: "sub:og:", // sub:og:<id>  -> delete origin <origin id>
  subOrigAdd: "sub:oa:", // sub:oa:<id>  -> add origin to subscription <id>
  subQr: "sub:q:", // sub:q:<id>   -> QR code of the subscription link
} as const;

/** A single inline button described as a label/data pair. */
export type Button = { text: string; data: string };

/** Builds an InlineKeyboard from rows of label/data pairs. */
export function kb(rows: Button[][]): InlineKeyboard {
  return InlineKeyboard.from(
    rows.map((row) => row.map((b) => InlineKeyboard.text(b.text, b.data))),
  );
}

/** The main menu keyboard. */
export function kbMenu(lang: string): InlineKeyboard {
  return kb([
    [{ text: t(lang, "btn.add"), data: CB.add }],
    [{ text: t(lang, "btn.subs"), data: CB.subs }],
    [
      { text: t(lang, "btn.lang"), data: CB.lang },
      { text: t(lang, "btn.help"), data: "help" },
    ],
  ]);
}

/** A keyboard with a single "back to menu" button. */
export function kbBack(lang: string): InlineKeyboard {
  return kb([[{ text: t(lang, "btn.back"), data: CB.menu }]]);
}

/** The language selection keyboard. */
export function kbLang(): InlineKeyboard {
  return kb([
    [{ text: "🇷🇺 Русский", data: CB.langPrefix + "ru" }],
    [{ text: "🇬🇧 English", data: CB.langPrefix + "en" }],
    [{ text: "⬅️ Back", data: CB.menu }],
  ]);
}

/** Lists subscriptions as buttons. */
export function kbSubs(subs: Subscription[], lang: string): InlineKeyboard {
  const rows: Button[][] = subs.map((sub) => [{ text: `▫️ ${sub.name}`, data: `${CB.sub}${sub.id}` }]);
  rows.push([{ text: t(lang, "btn.add"), data: CB.add }]);
  rows.push([{ text: t(lang, "btn.back"), data: CB.menu }]);
  return kb(rows);
}

/** The detail keyboard of one subscription. */
export function kbSub(id: number, lang: string): InlineKeyboard {
  return kb([
    [
      { text: t(lang, "btn.test"), data: `${CB.subTest}${id}` },
      { text: t(lang, "btn.devices"), data: `${CB.subDevices}${id}` },
    ],
    [
      { text: t(lang, "btn.origins"), data: `${CB.subOrigins}${id}` },
      { text: t(lang, "btn.qr"), data: `${CB.subQr}${id}` },
    ],
    [
      { text: t(lang, "btn.rename"), data: `${CB.subRename}${id}` },
      { text: t(lang, "btn.delete"), data: `${CB.subDelete}${id}` },
    ],
    [{ text: t(lang, "btn.back"), data: CB.subs }],
  ]);
}

/** Lists the origins with a settings button each, plus add and back. */
export function kbOrigins(sub: Subscription, lang: string): InlineKeyboard {
  const rows: Button[][] = sub.origins.map((o) => [
    { text: `${originStatusIcon(o)} ${defaultName(o.url)}`, data: `${CB.subOriginSettings}${o.id}` },
  ]);
  rows.push([{ text: t(lang, "btn.origin_add"), data: `${CB.subOrigAdd}${sub.id}` }]);
  rows.push([{ text: t(lang, "btn.back"), data: `${CB.sub}${sub.id}` }]);
  return kb(rows);
}

/** The settings keyboard of one origin: toggle, delete, back. */
export function kbOriginSettings(origin: Origin, subId: number, lang: string): InlineKeyboard {
  const toggle = origin.enabled ? t(lang, "btn.origin_disable") : t(lang, "btn.origin_enable");
  return kb([
    [{ text: toggle, data: `${CB.subOriginToggle}${origin.id}` }],
    [{ text: t(lang, "btn.delete"), data: `${CB.subOrigDelete}${origin.id}` }],
    [{ text: t(lang, "btn.back"), data: `${CB.subOrigins}${subId}` }],
  ]);
}

/** Asks for a delete confirmation. */
export function kbDeleteConfirm(id: number, lang: string): InlineKeyboard {
  return kb([
    [{ text: t(lang, "btn.delete_confirm"), data: `${CB.subDeleteDo}${id}` }],
    [{ text: t(lang, "btn.cancel"), data: `${CB.sub}${id}` }],
  ]);
}

/** Offers to cancel an interactive flow. */
export function kbCancelFlow(lang: string): InlineKeyboard {
  return kb([[{ text: t(lang, "btn.cancel"), data: "flow:cancel" }]]);
}

/** Offers to skip the name step of the add flow. */
export function kbAddNameSkip(lang: string): InlineKeyboard {
  return kb([
    [{ text: t(lang, "btn.skip"), data: CB.addNameSkip }],
    [{ text: t(lang, "btn.cancel"), data: "flow:cancel" }],
  ]);
}

/** Builds the detail message of one subscription. */
export function renderSubDetail(sub: Subscription, lang: string, baseUrl: string): string {
  return t(
    lang,
    "sub.detail",
    escapeHtml(sub.name),
    subscriptionLink(baseUrl, sub.token),
    sub.origins.length,
    renderOrigins(sub.origins, lang),
    formatTime(sub.createdAt, lang),
  );
}

/** Builds the message listing the origins of a subscription. */
export function renderOriginsScreen(sub: Subscription, lang: string): string {
  return `${t(lang, "origins.title", escapeHtml(sub.name), sub.origins.length)}\n\n${renderOrigins(sub.origins, lang)}`;
}

/** Builds the settings message of one origin. */
export function renderOriginSettings(origin: Origin, lang: string): string {
  return t(
    lang,
    "origin.settings",
    escapeHtml(origin.url),
    renderMode(origin, lang),
    renderStatus(origin, lang),
  );
}

/** Lists origins as "N. host - status" (never the HWID or full URL). */
export function renderOrigins(origins: Origin[], lang: string): string {
  return origins
    .map((o, i) => t(lang, "origin.line", i + 1, escapeHtml(defaultName(o.url)), renderStatus(o, lang)))
    .join("\n");
}

/** Describes how the HWID is passed to the origin. */
export function renderMode(o: Origin, lang: string): string {
  if (o.hwidMode === HWID_MODE_QUERY) {
    return t(lang, "sub.mode.query", escapeHtml(o.hwidParam));
  }
  return t(lang, "sub.mode.header", escapeHtml(o.hwidParam));
}

/** Localized on/off status of an origin. */
export function renderStatus(o: Origin, lang: string): string {
  return o.enabled ? t(lang, "origin.status_on") : t(lang, "origin.status_off");
}

/** Icon shown next to an origin in buttons. */
export function originStatusIcon(o: Origin): string {
  return o.enabled ? "✅" : "⏸";
}

/** Builds the public link served on our domain. */
export function subscriptionLink(baseUrl: string, token: string): string {
  return `${baseUrl}/s/${token}`;
}

/** Builds the message listing the devices of one subscription. */
export function renderDevices(name: string, lang: string, devices: Device[]): string {
  if (devices.length === 0) return t(lang, "devices.empty");

  let out = t(lang, "devices.title", escapeHtml(name), devices.length);
  devices.forEach((d, i) => {
    out += "\n\n";
    out += `${i + 1}. `;
    out += d.hwid !== "" ? t(lang, "devices.hwid", escapeHtml(d.hwid)) : t(lang, "devices.anon");
    const meta = deviceMeta(d);
    if (meta !== "") out += `\n${t(lang, "devices.device", escapeHtml(meta))}`;
    if (d.userAgent !== "") out += `\n${t(lang, "devices.ua", escapeHtml(d.userAgent))}`;
    if (d.ip !== "") out += `\n${t(lang, "devices.ip", escapeHtml(d.ip))}`;
    out += `\n${t(lang, "devices.stats", d.requests, formatTime(d.lastSeen, lang))}`;
  });
  return out;
}

/** Renders the model and OS of a device, skipping empty parts. */
function deviceMeta(d: Device): string {
  const os = `${d.os} ${d.osVersion}`.trim();
  const parts: string[] = [];
  if (d.model !== "") parts.push(d.model);
  if (os !== "") parts.push(os);
  return parts.join(" · ");
}

/** Escapes text for Telegram's HTML parse mode. */
export function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&#34;")
    .replace(/'/g, "&#39;");
}
