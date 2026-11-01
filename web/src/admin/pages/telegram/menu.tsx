// What subscribers see in the bot: the channel, the menu, the texts and the options.
import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useState } from "react";
import type { Schemas } from "../../../api/client";
import { CardHead } from "../../../components/layout";
import { Switch, SwitchRow } from "../../../components/switch";
import { Field, Segmented } from "../../../components/ui";
import { t, tMaybe } from "../../../i18n";
import { MarkdownHint, TgTextPreview } from "../telegram-text";
import { parseAdminIds } from "./admin-ids";
import { useSampleVars } from "./preview";
import { rise, slide, type Config, type MenuButton, type TextKey, type View } from "./shared";

/** Why the bot cannot check its channel, in the admin's words. */
export function channelErrorText(code: string): string {
  return tMaybe(`telegram.channel.err.${code}`) ?? t("telegram.channel.errOther", { error: code });
}

/**
 * The channel a subscriber must join before the menu, and the one the free plan may ask
 * for. The bot checks the channel as saved: an edit shows its state once it is.
 */
export function ChannelCard({ draft, setDraft, v }: { draft: Config; setDraft: (c: Config) => void; v: View }) {
  const ch = draft.channel;
  const set = (patch: Partial<Config["channel"]>) => setDraft({ ...draft, channel: { ...ch, ...patch } });
  const saved = v.config.channel;
  const state = !saved.chat || !v.running ? null : saved.chat !== ch.chat.trim() ? "checking" : v.channel_error ? "error" : "ok";
  return (
    <section {...rise(2)}>
      <CardHead title={t("telegram.channel.title")} sub={t("telegram.channel.sub")} />
      <div className="row-list">
        <SwitchRow label={t("telegram.channel.required")} sub={t("telegram.channel.requiredSub")} checked={ch.required} onChange={(on) => set({ required: on })} />
      </div>
      <div className="grid gap-x-3 pt-4 sm:grid-cols-2">
        <Field label={t("telegram.channel.chat")} htmlFor="tg-channel" hint={t("telegram.channel.chatHint")}>
          <input id="tg-channel" className="input mono" value={ch.chat} onChange={(e) => set({ chat: e.target.value })} placeholder="@my_channel" maxLength={64} autoComplete="off" spellCheck={false} />
        </Field>
        <Field label={t("telegram.channel.url")} htmlFor="tg-channel-url" hint={t("telegram.channel.urlHint")}>
          <input id="tg-channel-url" className="input mono" value={ch.url ?? ""} onChange={(e) => set({ url: e.target.value })} placeholder="https://t.me/+…" maxLength={256} autoComplete="off" spellCheck={false} />
        </Field>
      </div>
      {state === "error" ? (
        <div className="banner err" role="alert">
          <span>{channelErrorText(v.channel_error ?? "")}</span>
        </div>
      ) : state === "ok" ? (
        <p className="text-xs text-[var(--leaf-700)]" role="status">
          {t("telegram.channel.ok")}
        </p>
      ) : state === "checking" ? (
        <p className="text-xs text-[var(--ink-500)]">{t("telegram.channel.checking")}</p>
      ) : null}
    </section>
  );
}

/**
 * The accounts that may open /admin in the bot. The bot checks the list again on every
 * action, so an account taken out loses its access at once. The text is kept as typed while
 * it is edited, and the list is what it says.
 */
export function AdminsCard({ draft, setDraft }: { draft: Config; setDraft: (c: Config) => void }) {
  const ids = draft.admin_ids ?? [];
  const [text, setText] = useState(ids.join(", "));
  const { ids: parsed, bad } = parseAdminIds(text);
  const tooMany = parsed.length > MAX_ADMINS;
  const edit = (value: string) => {
    setText(value);
    setDraft({ ...draft, admin_ids: parseAdminIds(value).ids });
  };
  return (
    <section {...rise(3)}>
      <CardHead title={t("telegram.admins.title")} sub={t("telegram.admins.sub")} />
      <Field label={t("telegram.admins.ids")} htmlFor="tg-admins" hint={t("telegram.admins.idsHint", { max: MAX_ADMINS })}>
        <input
          id="tg-admins"
          className="input mono"
          value={text}
          onChange={(e) => edit(e.target.value)}
          placeholder="123456789, 987654321"
          inputMode="numeric"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={bad.length > 0 || tooMany}
        />
      </Field>
      {bad.length > 0 ? (
        <div className="banner err" role="alert">
          <span>{t("telegram.admins.bad", { list: bad.slice(0, 3).join(", ") })}</span>
        </div>
      ) : tooMany ? (
        <div className="banner err" role="alert">
          <span>{t("telegram.admins.tooMany", { max: MAX_ADMINS })}</span>
        </div>
      ) : (
        <p className="text-xs text-[var(--ink-500)]" role="status">
          {parsed.length === 0 ? t("telegram.admins.none") : t("telegram.admins.count", { n: parsed.length })}
        </p>
      )}
    </section>
  );
}

/** As many as the bot accepts (tgbot.MaxAdmins). */
const MAX_ADMINS = 20;

const ACTIONS = ["sub", "devices", "connect", "renew", "support", "app"] as const;

function actionLabel(a: string): string {
  return tMaybe(`telegram.action.${a}`) ?? a;
}

export function MenuCard({ draft, setDraft }: { draft: Config; setDraft: (c: Config) => void }) {
  const sample = useSampleVars();
  const set = (i: number, patch: Partial<MenuButton>) => setDraft({ ...draft, buttons: draft.buttons.map((b, j) => (j === i ? { ...b, ...patch } : b)) });
  const move = (i: number, d: -1 | 1) => {
    const list = [...draft.buttons];
    [list[i], list[i + d]] = [list[i + d]!, list[i]!];
    setDraft({ ...draft, buttons: list });
  };
  const add = (action: "url" | "page") => {
    setDraft({
      ...draft,
      buttons: [...draft.buttons, { id: `c${Date.now().toString(36)}`, action, label: action === "url" ? t("telegram.newLink") : t("telegram.newPage"), on: true, row: false, url: action === "url" ? "https://" : undefined, text: action === "page" ? "" : undefined }],
    });
  };
  const custom = (b: MenuButton) => !ACTIONS.includes(b.action as (typeof ACTIONS)[number]);
  const reduce = useReducedMotion();
  return (
    <section {...rise(1)}>
      <CardHead title={t("telegram.menu")} sub={t("telegram.menuSub")} />
      <ul className="flex flex-col gap-2">
        <AnimatePresence initial={false}>
          {draft.buttons.map((b, i) => (
            <motion.li
              key={b.id}
              className="panel-soft p-3"
              layout={!reduce}
              initial={reduce ? false : { opacity: 0, scale: 0.96 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={reduce ? { opacity: 0 } : { opacity: 0, scale: 0.96 }}
              transition={slide}
            >
              <div className="flex items-center gap-2">
                <Switch checked={b.on} label={t("telegram.showButton", { name: b.label })} onChange={(on) => set(i, { on })} />
                <input className="input h-10 min-w-0 flex-1" value={b.label} maxLength={40} onChange={(e) => set(i, { label: e.target.value })} aria-label={t("telegram.buttonLabel")} />
                <button type="button" className="icon-btn" disabled={i === 0} onClick={() => move(i, -1)} aria-label={t("telegram.moveUp")}>
                  <ArrowUp size={16} />
                </button>
                <button type="button" className="icon-btn" disabled={i === draft.buttons.length - 1} onClick={() => move(i, 1)} aria-label={t("telegram.moveDown")}>
                  <ArrowDown size={16} />
                </button>
                {custom(b) ? (
                  <button type="button" className="icon-btn" onClick={() => setDraft({ ...draft, buttons: draft.buttons.filter((_, j) => j !== i) })} aria-label={t("telegram.deleteButton", { name: b.label })}>
                    <Trash2 size={16} />
                  </button>
                ) : null}
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--ink-500)]">
                <span>{actionLabel(b.action)}</span>
                {i > 0 ? (
                  <label className="flex items-center gap-1.5">
                    <input type="checkbox" className="check" checked={b.row} onChange={(e) => set(i, { row: e.target.checked })} /> {t("telegram.sameRow")}
                  </label>
                ) : null}
              </div>
              {b.action === "url" ? <input className="input mono mt-2" value={b.url ?? ""} onChange={(e) => set(i, { url: e.target.value })} placeholder="https://… / tg://…" aria-label={t("telegram.buttonUrl")} /> : null}
              {b.action === "page" ? (
                <>
                  <textarea className="input mt-2" value={b.text ?? ""} maxLength={3000} onChange={(e) => set(i, { text: e.target.value })} placeholder={t("telegram.pagePlaceholder")} aria-label={t("telegram.pageText")} />
                  <div className="mt-1">
                    <MarkdownHint />
                  </div>
                  <TgTextPreview text={b.text ?? ""} vars={sample} label={t("telegram.md.preview")} />
                </>
              ) : null}
            </motion.li>
          ))}
        </AnimatePresence>
      </ul>
      <div className="mt-3 flex flex-wrap gap-2">
        <button type="button" className="chip-btn" onClick={() => add("url")} disabled={draft.buttons.length >= 20}>
          <Plus size={14} className="mr-1 inline" aria-hidden />
          {t("telegram.addLink")}
        </button>
        <button type="button" className="chip-btn" onClick={() => add("page")} disabled={draft.buttons.length >= 20}>
          <Plus size={14} className="mr-1 inline" aria-hidden />
          {t("telegram.addPage")}
        </button>
      </div>
    </section>
  );
}

const TEXTS: { key: TextKey; label: string }[] = [
  { key: "welcome", label: "telegram.text.welcome" },
  { key: "main", label: "telegram.text.main" },
  { key: "renew", label: "telegram.text.renew" },
  { key: "expiring", label: "telegram.text.expiring" },
  { key: "expired", label: "telegram.text.expired" },
  { key: "traffic_90", label: "telegram.text.traffic_90" },
  { key: "traffic_end", label: "telegram.text.traffic_end" },
  { key: "channel", label: "telegram.text.channel" },
];

export function TextsCard({ draft, setDraft, defaults }: { draft: Config; setDraft: (c: Config) => void; defaults: Schemas["Texts"] }) {
  // The text being edited shows under its field as the bot will send it.
  const [active, setActive] = useState<TextKey | null>(null);
  const sample = useSampleVars();
  return (
    <section {...rise(2)}>
      <CardHead title={t("telegram.texts")} sub={t("telegram.textsSub")} />
      <Field label={t("telegram.lang")} hint={t("telegram.langHint")}>
        <Segmented
          label={t("telegram.lang")}
          value={draft.lang}
          onChange={(lang) => setDraft({ ...draft, lang })}
          options={[
            { value: "ru", label: "Русский" },
            { value: "en", label: "English" },
          ]}
        />
      </Field>
      {TEXTS.map(({ key, label }) => (
        <Field key={key} label={tMaybe(label) ?? key} htmlFor={`tg-${key}`}>
          <>
            <textarea
              id={`tg-${key}`}
              className="input"
              rows={key === "main" || key === "welcome" ? 5 : 2}
              maxLength={3000}
              value={draft.texts[key]}
              placeholder={defaults[key]}
              onFocus={() => setActive(key)}
              onChange={(e) => setDraft({ ...draft, texts: { ...draft.texts, [key]: e.target.value } })}
            />
            {active === key ? <TgTextPreview text={draft.texts[key]} vars={sample} label={t("telegram.md.preview")} /> : null}
          </>
        </Field>
      ))}
      <Field label={t("telegram.freeButton")} htmlFor="tg-free-button" hint={t("telegram.freeButtonHint")}>
        <input
          id="tg-free-button"
          className="input"
          maxLength={40}
          value={draft.texts.free_button}
          placeholder={defaults.free_button}
          onChange={(e) => setDraft({ ...draft, texts: { ...draft.texts, free_button: e.target.value } })}
        />
      </Field>
      <p className="text-xs text-[var(--ink-500)]">{t("telegram.variables")}</p>
      <div className="mt-2">
        <MarkdownHint />
      </div>
    </section>
  );
}

const NOTICES: (keyof Schemas["Notify"])[] = ["expire_3d", "expire_1d", "expired", "traffic_90", "traffic_100"];

export function OptionsCard({ draft, setDraft, v }: { draft: Config; setDraft: (c: Config) => void; v: View }) {
  const row = (title: string, sub: string, on: boolean, change: (v: boolean) => void) => (
    <li key={title}>
      <SwitchRow label={title} sub={sub} checked={on} onChange={change} />
    </li>
  );
  return (
    <section {...rise(3)}>
      <CardHead title={t("telegram.options")} sub={t("telegram.optionsSub")} />
      <ul className="row-list">
        {row(t("telegram.miniApp"), v.mini_app_url ? t("telegram.miniAppSub") : t("telegram.miniAppNoCert"), draft.mini_app, (on) => setDraft({ ...draft, mini_app: on }))}
        {row(t("telegram.promoButton"), draft.mini_app ? t("telegram.promoButtonSub") : t("telegram.promoButtonNeedsApp"), draft.promo_button, (on) => setDraft({ ...draft, promo_button: on }))}
        {row(t("telegram.cleanChat"), t("telegram.cleanChatSub"), draft.clean_chat, (on) => setDraft({ ...draft, clean_chat: on }))}
        {row(t("telegram.quietNight"), t("telegram.quietNightSub"), draft.quiet_night, (on) => setDraft({ ...draft, quiet_night: on }))}
        {NOTICES.map((k) => row(t(`telegram.notice.${k}`), t("telegram.noticeSub"), draft.notify[k], (on) => setDraft({ ...draft, notify: { ...draft.notify, [k]: on } })))}
      </ul>
    </section>
  );
}
