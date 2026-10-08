// Telegram bot: commands, inline callbacks and interactive flows (grammY).

import { Bot, type Context, type InlineKeyboard, InputFile } from "grammy";
import type { Config } from "../config";
import { generateHWID } from "../hwid";
import { normalize, t } from "../i18n";
import { log } from "../log";
import type { OriginClient } from "../origin";
import { renderQrPng } from "../qr";
import { HWID_MODE_HEADER, LastOriginError, NotFoundError, type Store, type Subscription } from "../store";
import { defaultName, detectHwidMode, isHttpUrl } from "../url";
import {
  CB,
  escapeHtml,
  kbAddNameSkip,
  kbBack,
  kbCancelFlow,
  kbDeleteConfirm,
  kbLang,
  kbMenu,
  kbOriginSettings,
  kbOrigins,
  kbSub,
  kbSubs,
  renderDevices,
  renderOriginSettings,
  renderOrigins,
  renderOriginsScreen,
  renderSubDetail,
  subscriptionLink,
} from "./views";

// Interactive flow kinds (in-memory, per user).
type FlowKind = "add_url" | "add_name" | "rename" | "add_origin";

/** State of an interactive multi-step action. */
interface Flow {
  kind: FlowKind;
  subId?: number;
  url?: string;
  hwidMode?: string;
  hwidParam?: string;
}

const MAX_NAME_RUNES = 64;

/** Bot is the Telegram bot backed by the store and the origin client. */
export class BotService {
  private readonly tg: Bot;
  private readonly defaultLang: string;
  private readonly flows = new Map<number, Flow>();

  constructor(
    private readonly store: Store,
    private readonly origin: OriginClient,
    private readonly cfg: Config,
  ) {
    this.defaultLang = normalize(cfg.defaultLocale);
    this.tg = new Bot(cfg.telegramBotToken);
    this.register();
  }

  /** Registers all handlers. */
  private register(): void {
    this.tg.command("start", (ctx) => this.cmdStart(ctx));
    this.tg.command("help", (ctx) => this.cmdHelp(ctx));
    this.tg.command("lang", (ctx) => this.cmdLang(ctx));
    this.tg.command("add", (ctx) => this.cmdAdd(ctx));
    this.tg.command("subs", (ctx) => this.cmdSubs(ctx));
    this.tg.command("cancel", (ctx) => this.cmdCancel(ctx));

    this.tg.on("message:text", (ctx) => this.onMessage(ctx));
    this.tg.on("callback_query:data", (ctx) => this.onCallback(ctx));
  }

  /** Starts long polling (resolves when the bot is stopped). */
  async run(): Promise<void> {
    this.tg.catch((err) => {
      log.error("bot: handler error", { update: err.ctx.update.update_id, err: err.error });
    });
    log.info("bot: polling started");
    await this.tg.start();
  }

  /** Stops long polling. */
  stop(): void {
    this.tg.stop();
  }

  // --- Commands -------------------------------------------------------------

  private async cmdStart(ctx: Context): Promise<void> {
    const lang = await this.langOf(ctx);
    await this.sendHtml(ctx, `${t(lang, "start.welcome")}\n\n${t(lang, "start.menu")}`, kbMenu(lang));
  }

  private async cmdHelp(ctx: Context): Promise<void> {
    const lang = await this.langOf(ctx);
    const example = subscriptionLink(this.cfg.publicBaseUrl, "TOKEN");
    await this.sendHtml(ctx, t(lang, "help.text", example), kbMenu(lang));
  }

  private async cmdLang(ctx: Context): Promise<void> {
    const lang = await this.langOf(ctx);
    await this.sendHtml(ctx, t(lang, "lang.choose"), kbLang());
  }

  private async cmdAdd(ctx: Context): Promise<void> {
    const lang = await this.langOf(ctx);
    const id = ctx.from?.id;
    if (id === undefined) return;
    this.startFlow(id, { kind: "add_url" });
    await this.sendHtml(ctx, t(lang, "add.ask_url"), kbCancelFlow(lang));
  }

  private async cmdSubs(ctx: Context): Promise<void> {
    const lang = await this.langOf(ctx);
    const uid = ctx.from?.id;
    if (uid === undefined) return;
    let subs: Subscription[];
    try {
      subs = await this.store.listSubscriptions(uid);
    } catch (err) {
      log.error("bot: list subscriptions failed", { err });
      await this.sendHtml(ctx, t(lang, "err.db"), kbMenu(lang));
      return;
    }
    if (subs.length === 0) {
      await this.sendHtml(ctx, t(lang, "subs.empty"), kbMenu(lang));
      return;
    }
    await this.sendHtml(ctx, t(lang, "subs.title", subs.length), kbSubs(subs, lang));
  }

  private async cmdCancel(ctx: Context): Promise<void> {
    const lang = await this.langOf(ctx);
    const id = ctx.from?.id;
    if (id !== undefined) this.clearFlow(id);
    await this.sendHtml(ctx, t(lang, "flow.canceled"), kbMenu(lang));
  }

  // --- Free-text messages ---------------------------------------------------

  private async onMessage(ctx: Context): Promise<void> {
    const lang = await this.langOf(ctx);
    const id = ctx.from?.id;
    if (id === undefined) return;
    const message = ctx.message;
    if (message === undefined || message.text === undefined) return;
    const text = message.text.trim();
    if (text === "") return;

    const flow = this.peekFlow(id);
    if (flow === undefined) {
      // Ignore unknown slash commands, nudge everything else.
      if (!text.startsWith("/")) {
        await this.sendHtml(ctx, t(lang, "flow.unknown"), kbMenu(lang));
      }
      return;
    }

    if (isCancelText(text)) {
      this.clearFlow(id);
      await this.sendHtml(ctx, t(lang, "flow.canceled"), kbMenu(lang));
      return;
    }

    switch (flow.kind) {
      case "add_url":
        await this.flowAddUrl(ctx, id, lang, text);
        return;
      case "add_name":
        await this.flowAddName(ctx, id, lang, text);
        return;
      case "rename":
        await this.flowRename(ctx, id, lang, text);
        return;
      case "add_origin":
        await this.flowAddOrigin(ctx, id, lang, text);
        return;
      default:
        this.clearFlow(id);
        await this.sendHtml(ctx, t(lang, "flow.unknown"), kbMenu(lang));
    }
  }

  private async flowAddUrl(ctx: Context, uid: number, lang: string, text: string): Promise<void> {
    const rawUrl = text.trim();
    if (!isHttpUrl(rawUrl)) {
      await this.sendHtml(ctx, t(lang, "add.invalid_url"), kbCancelFlow(lang));
      return;
    }
    const { mode, param } = detectHwidMode(rawUrl);
    this.startFlow(uid, { kind: "add_name", url: rawUrl, hwidMode: mode, hwidParam: param });
    await this.sendHtml(ctx, t(lang, "add.ask_name"), kbAddNameSkip(lang));
  }

  private async flowAddName(ctx: Context, uid: number, lang: string, text: string): Promise<void> {
    const flow = this.takeFlow(uid);
    if (flow === undefined) return;
    const name = text.trim();
    const length = Array.from(name).length;
    if (length === 0 || length > MAX_NAME_RUNES) {
      this.startFlow(uid, flow);
      await this.sendHtml(ctx, t(lang, "add.invalid_name"), kbAddNameSkip(lang));
      return;
    }
    await this.createSubscription(ctx, uid, lang, flow, name);
  }

  /** Finishes the add flow with a name derived from the origin host. */
  private async completeAddWithoutName(ctx: Context, uid: number, lang: string): Promise<void> {
    const flow = this.takeFlow(uid);
    if (flow === undefined) return;
    if (flow.kind !== "add_name") {
      // The URL step is not finished yet: keep the flow and re-ask.
      this.startFlow(uid, flow);
      await this.sendHtml(ctx, t(lang, "add.ask_url"), kbCancelFlow(lang));
      return;
    }
    await this.createSubscription(ctx, uid, lang, flow, defaultName(flow.url ?? ""));
  }

  /** Generates the HWID once and stores the subscription. */
  private async createSubscription(ctx: Context, uid: number, lang: string, flow: Flow, name: string): Promise<void> {
    if (flow.url === undefined) return;
    const hwid = generateHWID();
    let sub: Subscription;
    try {
      sub = await this.store.createSubscription({
        userId: uid,
        name,
        origins: [
          {
            url: flow.url,
            hwid,
            hwidMode: flow.hwidMode ?? HWID_MODE_HEADER,
            hwidParam: flow.hwidParam ?? "x-hwid",
          },
        ],
      });
    } catch (err) {
      log.error("bot: create subscription failed", { err });
      await this.sendHtml(ctx, t(lang, "err.db"), kbMenu(lang));
      return;
    }
    await this.sendHtml(
      ctx,
      t(lang, "add.created", escapeHtml(name), subscriptionLink(this.cfg.publicBaseUrl, sub.token), renderOrigins(sub.origins, lang)),
      kbSub(sub.id, lang),
    );
  }

  /** Attaches one more origin to the subscription of the flow. */
  private async flowAddOrigin(ctx: Context, uid: number, lang: string, text: string): Promise<void> {
    const rawUrl = text.trim();
    if (!isHttpUrl(rawUrl)) {
      await this.sendHtml(ctx, t(lang, "add.invalid_url"), kbCancelFlow(lang));
      return;
    }
    const flow = this.takeFlow(uid);
    if (flow === undefined || flow.subId === undefined) return;

    const { mode, param } = detectHwidMode(rawUrl);
    try {
      await this.store.addOrigin(uid, flow.subId, {
        url: rawUrl,
        hwid: generateHWID(),
        hwidMode: mode,
        hwidParam: param,
      });
    } catch (err) {
      if (err instanceof NotFoundError) {
        await this.sendHtml(ctx, t(lang, "sub.not_found"), kbMenu(lang));
        return;
      }
      log.error("bot: add origin failed", { err });
      await this.sendHtml(ctx, t(lang, "err.db"), kbMenu(lang));
      return;
    }

    const sub = await this.loadSubForMessage(ctx, uid, flow.subId, lang);
    if (sub === null) return;
    await this.sendHtml(ctx, `${t(lang, "origin.added")}\n\n${renderOriginsScreen(sub, lang)}`, kbOrigins(sub, lang));
  }

  /** Saves a new subscription name. */
  private async flowRename(ctx: Context, uid: number, lang: string, text: string): Promise<void> {
    const flow = this.takeFlow(uid);
    if (flow === undefined || flow.subId === undefined) return;
    const name = text.trim();
    const length = Array.from(name).length;
    if (length === 0 || length > MAX_NAME_RUNES) {
      this.startFlow(uid, flow);
      await this.sendHtml(ctx, t(lang, "add.invalid_name"), kbCancelFlow(lang));
      return;
    }
    try {
      await this.store.renameSubscription(uid, flow.subId, name);
    } catch (err) {
      if (err instanceof NotFoundError) {
        await this.sendHtml(ctx, t(lang, "sub.not_found"), kbMenu(lang));
        return;
      }
      log.error("bot: rename subscription failed", { err });
      await this.sendHtml(ctx, t(lang, "err.db"), kbMenu(lang));
      return;
    }
    const detail = await this.detailBlock(uid, flow.subId, lang);
    await this.sendHtml(ctx, t(lang, "sub.renamed", escapeHtml(name)) + detail, kbSub(flow.subId, lang));
  }

  // --- Callbacks ------------------------------------------------------------

  private async onCallback(ctx: Context): Promise<void> {
    const data = ctx.callbackQuery?.data;
    if (data === undefined) return;
    const lang = await this.langOf(ctx);
    const uid = ctx.from?.id;
    if (uid === undefined) return;

    if (data === CB.menu) {
      await this.answer(ctx);
      await this.editHtml(ctx, t(lang, "start.menu"), kbMenu(lang));
      return;
    }
    if (data === "help") {
      await this.answer(ctx);
      await this.editHtml(ctx, t(lang, "help.text", subscriptionLink(this.cfg.publicBaseUrl, "TOKEN")), kbMenu(lang));
      return;
    }
    if (data === CB.subs) {
      await this.answer(ctx);
      await this.editSubsList(ctx, uid, lang);
      return;
    }
    if (data === CB.add) {
      await this.answer(ctx);
      this.startFlow(uid, { kind: "add_url" });
      await this.editHtml(ctx, t(lang, "add.ask_url"), kbCancelFlow(lang));
      return;
    }
    if (data === CB.addNameSkip) {
      await this.answer(ctx);
      await this.completeAddWithoutName(ctx, uid, lang);
      return;
    }
    if (data === CB.lang) {
      await this.answer(ctx);
      await this.editHtml(ctx, t(lang, "lang.choose"), kbLang());
      return;
    }
    if (data === CB.langPrefix + "ru" || data === CB.langPrefix + "en") {
      const code = data.slice(CB.langPrefix.length);
      await this.answer(ctx);
      try {
        await this.store.setLang(uid, code);
        await this.editHtml(ctx, t(code, "lang.set"), kbMenu(code));
      } catch (err) {
        log.error("bot: set lang failed", { err });
      }
      return;
    }
    if (data === "flow:cancel") {
      await this.answer(ctx, t(lang, "flow.canceled"));
      this.clearFlow(uid);
      await this.editHtml(ctx, t(lang, "start.menu"), kbMenu(lang));
      return;
    }

    await this.dispatchSubCallback(ctx, uid, lang, data);
  }

  /** Handles subscription-scoped button presses (most specific prefixes first). */
  private async dispatchSubCallback(ctx: Context, uid: number, lang: string, data: string): Promise<void> {
    let id: number;

    if ((id = parseId(CB.subDeleteDo, data)) > 0) {
      await this.actDelete(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subDelete, data)) > 0) {
      const sub = await this.loadSub(ctx, uid, lang, id);
      if (sub === null) return;
      await this.answer(ctx);
      await this.editHtml(ctx, t(lang, "sub.delete_confirm", escapeHtml(sub.name)), kbDeleteConfirm(id, lang));
      return;
    }
    if ((id = parseId(CB.subRename, data)) > 0) {
      if ((await this.loadSub(ctx, uid, lang, id)) === null) return;
      await this.answer(ctx);
      this.startFlow(uid, { kind: "rename", subId: id });
      await this.editHtml(ctx, t(lang, "sub.rename.ask"), kbCancelFlow(lang));
      return;
    }
    if ((id = parseId(CB.subTest, data)) > 0) {
      await this.actTest(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subDevices, data)) > 0) {
      await this.actDevices(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subOriginToggle, data)) > 0) {
      await this.actToggleOrigin(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subOriginSettings, data)) > 0) {
      await this.actOriginSettings(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subOrigDelete, data)) > 0) {
      await this.actDeleteOrigin(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subOrigAdd, data)) > 0) {
      await this.actAddOriginStart(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subQr, data)) > 0) {
      await this.actQr(ctx, uid, lang, id);
      return;
    }
    if ((id = parseId(CB.subOrigins, data)) > 0) {
      const sub = await this.loadSub(ctx, uid, lang, id);
      if (sub === null) return;
      await this.answer(ctx);
      await this.editHtml(ctx, renderOriginsScreen(sub, lang), kbOrigins(sub, lang));
      return;
    }
    if ((id = parseId(CB.sub, data)) > 0) {
      const sub = await this.loadSub(ctx, uid, lang, id);
      if (sub === null) return;
      await this.answer(ctx);
      await this.editHtml(ctx, renderSubDetail(sub, lang, this.cfg.publicBaseUrl), kbSub(id, lang));
      return;
    }

    await this.answer(ctx);
  }

  // --- Callback actions -----------------------------------------------------

  /** Fetches every origin with its HWID and reports the outcome of each one. */
  private async actTest(ctx: Context, uid: number, lang: string, id: number): Promise<void> {
    const sub = await this.loadSub(ctx, uid, lang, id);
    if (sub === null) return;
    await this.answer(ctx, `${t(lang, "btn.test")}…`);

    const signal = AbortSignal.timeout(this.cfg.originTimeoutMs);
    const lines: string[] = [];
    for (const o of sub.origins.filter((origin) => origin.enabled)) {
      const host = escapeHtml(defaultName(o.url));
      try {
        const res = await this.origin.fetch(o, signal);
        lines.push(t(lang, "sub.test_origin_ok", host, res.statusCode, res.body.byteLength));
      } catch (err) {
        lines.push(t(lang, "sub.test_origin_fail", host, escapeHtml((err as Error).message)));
      }
    }
    await this.sendHtml(ctx, t(lang, "sub.test_result", escapeHtml(sub.name), lines.join("\n")), kbSub(id, lang));
  }

  /** Sends the public subscription link as a QR code photo. */
  private async actQr(ctx: Context, uid: number, lang: string, id: number): Promise<void> {
    const sub = await this.loadSub(ctx, uid, lang, id);
    if (sub === null) return;
    await this.answer(ctx);
    const link = subscriptionLink(this.cfg.publicBaseUrl, sub.token);
    let png: Uint8Array;
    try {
      png = await renderQrPng(link);
    } catch (err) {
      log.error("bot: render qr failed", { id, err });
      await this.sendHtml(ctx, t(lang, "err.generic"), kbSub(id, lang));
      return;
    }
    try {
      await ctx.replyWithPhoto(new InputFile(png, "subscription-qr.png"), {
        caption: t(lang, "sub.qr_caption", escapeHtml(sub.name), link),
        parse_mode: "HTML",
        reply_markup: kbSub(id, lang),
      });
    } catch (err) {
      log.warn("bot: send qr failed", { id, err });
    }
  }

  /** Asks for the URL of one more origin of the subscription. */
  private async actAddOriginStart(ctx: Context, uid: number, lang: string, id: number): Promise<void> {
    const sub = await this.loadSub(ctx, uid, lang, id);
    if (sub === null) return;
    await this.answer(ctx);
    this.startFlow(uid, { kind: "add_origin", subId: sub.id });
    await this.editHtml(ctx, t(lang, "origin.ask_url", escapeHtml(sub.name)), kbCancelFlow(lang));
  }

  /** Opens the settings screen of one origin. */
  private async actOriginSettings(ctx: Context, uid: number, lang: string, originId: number): Promise<void> {
    let loaded;
    try {
      loaded = await this.store.getOrigin(uid, originId);
    } catch (err) {
      await this.answer(ctx);
      if (err instanceof NotFoundError) {
        await this.editHtml(ctx, t(lang, "sub.not_found"), kbBack(lang));
      } else {
        log.error("bot: load origin failed", { id: originId, err });
        await this.sendHtml(ctx, t(lang, "err.db"));
      }
      return;
    }
    await this.answer(ctx);
    await this.editHtml(
      ctx,
      renderOriginSettings(loaded.origin, lang),
      kbOriginSettings(loaded.origin, loaded.subscriptionId, lang),
    );
  }

  /** Enables or disables one origin and re-renders its settings. */
  private async actToggleOrigin(ctx: Context, uid: number, lang: string, originId: number): Promise<void> {
    let loaded;
    try {
      loaded = await this.store.getOrigin(uid, originId);
    } catch (err) {
      await this.answer(ctx);
      if (err instanceof NotFoundError) {
        await this.editHtml(ctx, t(lang, "sub.not_found"), kbBack(lang));
      } else {
        log.error("bot: load origin failed", { id: originId, err });
        await this.sendHtml(ctx, t(lang, "err.db"));
      }
      return;
    }

    const enabled = !loaded.origin.enabled;
    try {
      await this.store.setOriginEnabled(uid, originId, enabled);
    } catch (err) {
      await this.answer(ctx);
      if (err instanceof NotFoundError) {
        await this.editHtml(ctx, t(lang, "sub.not_found"), kbBack(lang));
      } else {
        log.error("bot: toggle origin failed", { id: originId, err });
        await this.sendHtml(ctx, t(lang, "err.db"));
      }
      return;
    }

    await this.answer(ctx, t(lang, enabled ? "origin.enabled" : "origin.disabled"));
    const origin = { ...loaded.origin, enabled };
    await this.editHtml(
      ctx,
      renderOriginSettings(origin, lang),
      kbOriginSettings(origin, loaded.subscriptionId, lang),
    );
  }

  /** Removes one origin. A subscription keeps at least one. */
  private async actDeleteOrigin(ctx: Context, uid: number, lang: string, originId: number): Promise<void> {
    let subId: number;
    try {
      subId = await this.store.deleteOrigin(uid, originId);
    } catch (err) {
      if (err instanceof LastOriginError) {
        await this.answer(ctx, t(lang, "origin.last"), true);
        return;
      }
      if (err instanceof NotFoundError) {
        await this.answer(ctx);
        await this.editHtml(ctx, t(lang, "sub.not_found"), kbBack(lang));
        return;
      }
      log.error("bot: delete origin failed", { id: originId, err });
      await this.answer(ctx);
      await this.sendHtml(ctx, t(lang, "err.db"));
      return;
    }

    let sub: Subscription;
    try {
      sub = await this.store.getSubscription(uid, subId);
    } catch (err) {
      log.error("bot: load subscription failed", { id: subId, err });
      await this.answer(ctx);
      await this.sendHtml(ctx, t(lang, "err.db"));
      return;
    }
    await this.answer(ctx, t(lang, "origin.deleted"));
    await this.editHtml(ctx, renderOriginsScreen(sub, lang), kbOrigins(sub, lang));
  }

  /** Lists the devices that fetched the subscription. */
  private async actDevices(ctx: Context, uid: number, lang: string, id: number): Promise<void> {
    const sub = await this.loadSub(ctx, uid, lang, id);
    if (sub === null) return;
    let devices;
    try {
      devices = await this.store.listDevices(id, 30);
    } catch (err) {
      log.error("bot: list devices failed", { err });
      await this.answer(ctx);
      await this.sendHtml(ctx, t(lang, "err.db"));
      return;
    }
    await this.answer(ctx);
    await this.editHtml(ctx, renderDevices(sub.name, lang, devices), kbSub(id, lang));
  }

  /** Removes a subscription after confirmation. */
  private async actDelete(ctx: Context, uid: number, lang: string, id: number): Promise<void> {
    let sub: Subscription;
    try {
      sub = await this.store.getSubscription(uid, id);
    } catch (err) {
      await this.answer(ctx);
      if (err instanceof NotFoundError) {
        await this.editHtml(ctx, t(lang, "sub.not_found"), kbBack(lang));
      } else {
        log.error("bot: load subscription failed", { id, err });
        await this.sendHtml(ctx, t(lang, "err.db"));
      }
      return;
    }
    try {
      await this.store.deleteSubscription(uid, id);
    } catch (err) {
      log.error("bot: delete subscription failed", { id, err });
      await this.answer(ctx);
      await this.sendHtml(ctx, t(lang, "err.db"));
      return;
    }
    await this.answer(ctx, t(lang, "sub.deleted", escapeHtml(sub.name)));
    await this.editSubsList(ctx, uid, lang);
  }

  /** Renders the subscription list into the current message. */
  private async editSubsList(ctx: Context, uid: number, lang: string): Promise<void> {
    let subs: Subscription[];
    try {
      subs = await this.store.listSubscriptions(uid);
    } catch (err) {
      log.error("bot: list subscriptions failed", { err });
      await this.editHtml(ctx, t(lang, "err.db"), kbBack(lang));
      return;
    }
    if (subs.length === 0) {
      await this.editHtml(ctx, t(lang, "subs.empty"), kbMenu(lang));
      return;
    }
    await this.editHtml(ctx, t(lang, "subs.title", subs.length), kbSubs(subs, lang));
  }

  // --- Helpers --------------------------------------------------------------

  /** Fetches a subscription owned by the user, reporting "not found" into the current message. */
  private async loadSub(ctx: Context, uid: number, lang: string, id: number): Promise<Subscription | null> {
    try {
      return await this.store.getSubscription(uid, id);
    } catch (err) {
      await this.answer(ctx);
      if (err instanceof NotFoundError) {
        await this.editHtml(ctx, t(lang, "sub.not_found"), kbBack(lang));
      } else {
        log.error("bot: load subscription failed", { id, err });
        await this.sendHtml(ctx, t(lang, "err.db"));
      }
      return null;
    }
  }

  /** Like loadSub, but reports problems as a new message instead of editing. */
  private async loadSubForMessage(ctx: Context, uid: number, id: number, lang: string): Promise<Subscription | null> {
    try {
      return await this.store.getSubscription(uid, id);
    } catch (err) {
      if (err instanceof NotFoundError) {
        await this.sendHtml(ctx, t(lang, "sub.not_found"), kbMenu(lang));
      } else {
        log.error("bot: load subscription failed", { id, err });
        await this.sendHtml(ctx, t(lang, "err.db"), kbMenu(lang));
      }
      return null;
    }
  }

  /** Renders the detail card of a subscription as an appendix. */
  private async detailBlock(uid: number, subId: number, lang: string): Promise<string> {
    try {
      const sub = await this.store.getSubscription(uid, subId);
      return "\n\n" + renderSubDetail(sub, lang, this.cfg.publicBaseUrl);
    } catch {
      return "";
    }
  }

  /** Loads the sender's profile, creating it on first contact. */
  private async user(ctx: Context) {
    const id = ctx.from?.id;
    if (id === undefined) return null;
    await this.store.upsertUser(id, this.defaultLang);
    return await this.store.getUser(id);
  }

  /** The interface language of the sender. */
  private async langOf(ctx: Context): Promise<string> {
    const user = await this.user(ctx);
    return user?.lang ?? this.defaultLang;
  }

  private async sendHtml(ctx: Context, text: string, keyboard?: InlineKeyboard): Promise<void> {
    try {
      await ctx.reply(text, { parse_mode: "HTML", reply_markup: keyboard });
    } catch (err) {
      log.warn("bot: reply failed", { err });
    }
  }

  private async editHtml(ctx: Context, text: string, keyboard?: InlineKeyboard): Promise<void> {
    try {
      await ctx.editMessageText(text, { parse_mode: "HTML", reply_markup: keyboard });
    } catch {
      // "message is not modified" and friends are not fatal.
    }
  }

  /** Acknowledges a callback query; non-empty text shows as a toast or alert. */
  private async answer(ctx: Context, text = "", showAlert = false): Promise<void> {
    try {
      await ctx.answerCallbackQuery(text === "" ? undefined : { text, show_alert: showAlert });
    } catch (err) {
      log.warn("bot: answerCallbackQuery failed", { err });
    }
  }

  private startFlow(userId: number, flow: Flow): void {
    this.flows.set(userId, flow);
  }

  /** Returns and clears the active flow of the sender. */
  private takeFlow(userId: number): Flow | undefined {
    const flow = this.flows.get(userId);
    this.flows.delete(userId);
    return flow;
  }

  private peekFlow(userId: number): Flow | undefined {
    return this.flows.get(userId);
  }

  private clearFlow(userId: number): void {
    this.flows.delete(userId);
  }
}

/** Extracts a positive numeric id from a "prefix<id>" callback payload. */
function parseId(prefix: string, data: string): number {
  if (!data.startsWith(prefix)) return 0;
  const raw = data.slice(prefix.length);
  if (!/^\d+$/.test(raw)) return 0;
  const id = Number(raw);
  return Number.isSafeInteger(id) && id > 0 ? id : 0;
}

/** Matches localized cancel commands typed as plain text. */
function isCancelText(text: string): boolean {
  switch (text.toLowerCase()) {
    case "/cancel":
    case "cancel":
    case "отмена":
    case "✖️ отмена":
    case "✖️ cancel":
      return true;
    default:
      return false;
  }
}
