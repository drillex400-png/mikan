package tgbot

import (
	"context"
	"fmt"
	"html"
	"strings"

	"mikan/internal/panel/events"
	"mikan/internal/panel/growth"
	"mikan/internal/tgapi"
)

// The referral programme in the bot. It sits beside the subscribers' screens and does not
// change them: /invite is its own command, and a /start code that is not an invitation goes
// on to the screens as before. The money logic is package growth; this file is Telegram.

// Growther is what a Backend offers when it can run the programme (Remote does).
type Growther interface{ Growth() *growth.Service }

func (b *Bot) growth() *growth.Service {
	if g, ok := b.d.Backend.(Growther); ok {
		return g.Growth()
	}
	return nil
}

type growthWords struct {
	title, how, link, friends, off, bonusInviter, bonusFriend, share, cmdInvite string
}

var growthRU = growthWords{
	title:        "🎁 <b>Приглашайте друзей</b>",
	how:          "Друг запускает бота по вашей ссылке и оплачивает первую подписку. Вы оба получаете дни в подарок.",
	link:         "Ваша ссылка",
	friends:      "Оплатили друзей",
	off:          "Реферальная программа сейчас отключена.",
	bonusInviter: "🎉 Ваш друг оплатил подписку. Вам начислено дней: <b>%d</b>.",
	bonusFriend:  "🎁 Подарок за приглашение: <b>%d</b> дн. к вашей подписке.",
	share:        "Поделиться",
	cmdInvite:    "Пригласить друзей",
}

var growthEN = growthWords{
	title:        "🎁 <b>Invite friends</b>",
	how:          "A friend starts the bot from your link and pays for the first subscription. You both get days as a gift.",
	link:         "Your link",
	friends:      "Friends who paid",
	off:          "The referral programme is switched off now.",
	bonusInviter: "🎉 Your friend paid for a subscription. Days added to yours: <b>%d</b>.",
	bonusFriend:  "🎁 Invitation gift: <b>%d</b> days added to your subscription.",
	share:        "Share",
	cmdInvite:    "Invite friends",
}

func growthWordsFor(lang string) *growthWords {
	if lang == "en" {
		return &growthEN
	}
	return &growthRU
}

// inviteOn says whether the programme runs now, so /invite is worth listing.
func (b *Bot) inviteOn(ctx context.Context) bool {
	g := b.growth()
	if g == nil {
		return false
	}
	st, err := g.Settings(ctx)
	return err == nil && st.Enabled
}

// commandMenu is the command list of a chat: /start, /invite while the programme runs, and
// /admin for an admin. The second result is a key that changes when the list does, so a
// cached menu is set again.
func (b *Bot) commandMenu(ctx context.Context, admin bool) ([]tgapi.Command, string) {
	cfg := b.config(ctx)
	w := growthWordsFor(cfg.Lang)
	cs := []tgapi.Command{{Name: "start", Desc: wordsFor(cfg.Lang).commands}}
	key := cfg.Lang
	if b.inviteOn(ctx) {
		cs = append(cs, tgapi.Command{Name: "invite", Desc: w.cmdInvite})
		key += "+invite"
	}
	if admin {
		cs = append(cs, tgapi.Command{Name: "admin", Desc: admWordsFor(cfg.Lang).cmdAdmin})
	}
	return cs, key
}

// inviteLink is the link a friend opens; "" until the bot's name is known.
func (b *Bot) inviteLink(ctx context.Context, tgID int64) string {
	me, err := b.d.Client.Me(ctx)
	if err != nil || me.Username == "" {
		return ""
	}
	return "https://t.me/" + me.Username + "?start=" + growth.Link(tgID)
}

// inviteMessage handles /invite. It returns true when the message was this command.
func (b *Bot) inviteMessage(ctx context.Context, m *tgapi.Message) bool {
	cmd, _, _ := strings.Cut(strings.TrimSpace(m.Text), " ")
	if cmd, _, _ = strings.Cut(cmd, "@"); cmd != "/invite" || m.Chat.Type != "private" {
		return false
	}
	g := b.growth()
	if g == nil {
		return false
	}
	chat := m.Chat.ID
	w := growthWordsFor(b.config(ctx).Lang)
	st, err := g.Settings(ctx)
	if err != nil {
		b.d.Log.Warn("growth: settings", "err", err)
		return false
	}
	if !st.Enabled {
		b.d.Outbox.Reply(chat, "", 1, func(ctx context.Context, c *tgapi.Client) error {
			_, err := c.Send(ctx, chat, w.off, nil, false)
			return err
		})
		return true
	}
	link := b.inviteLink(ctx, chat)
	paid, _ := g.Invited(ctx, chat)
	var sb strings.Builder
	sb.WriteString(w.title + "\n\n" + html.EscapeString(w.how))
	fmt.Fprintf(&sb, "\n\n<blockquote>%s: <b>%d</b>", w.friends, paid)
	if link != "" {
		fmt.Fprintf(&sb, "\n%s:\n<code>%s</code>", w.link, html.EscapeString(link))
	}
	sb.WriteString("</blockquote>")
	var kb *tgapi.Keyboard
	if link != "" {
		share := "https://t.me/share/url?url=" + urlQuery(link)
		kb = &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{{{Text: "📤 " + w.share, URL: share, Style: tgapi.StylePrimary}}}}
	}
	b.d.Outbox.Reply(chat, "", 1, func(ctx context.Context, c *tgapi.Client) error {
		_, err := c.Send(ctx, chat, sb.String(), kb, false)
		return err
	})
	return true
}

func urlQuery(s string) string {
	r := strings.NewReplacer("%", "%25", "&", "%26", "?", "%3F", "=", "%3D", "#", "%23", " ", "%20", ":", "%3A", "/", "%2F")
	return r.Replace(s)
}

// startInvitation notes who a /start code r_<id> names. It reports whether the code was an
// invitation, so the caller can leave it out of the subscription-code path.
func (b *Bot) startInvitation(ctx context.Context, friend int64, code string) bool {
	if _, ok := growth.ParseCode(code); !ok {
		return false
	}
	g := b.growth()
	if g == nil {
		return true // still an invitation, not a subscription code
	}
	acc, err := b.d.Backend.Context(ctx, friend)
	if err != nil {
		b.d.Log.Warn("growth: account", "err", err)
		return true
	}
	if _, err := g.Accept(ctx, friend, code, len(acc.Links) > 0); err != nil {
		b.d.Log.Warn("growth: accept", "err", err)
	}
	return true
}

// rewardFirstPayment is called for payment.paid. Only the payment that made a new
// subscription counts; the rest of the payments are not the programme's business.
func (b *Bot) rewardFirstPayment(ctx context.Context, e events.Event, p events.Payment) error {
	if !p.Created || p.Kind != "new" || e.TgID == 0 || e.UserID == 0 {
		return nil
	}
	g := b.growth()
	if g == nil {
		return nil
	}
	inviter, invited, err := g.InviterOf(ctx, e.TgID)
	if err != nil || !invited {
		return err
	}
	acc, err := b.d.Backend.Context(ctx, inviter)
	if err != nil {
		return err
	}
	var inviterSub int64
	for _, u := range acc.Links {
		if u.Status != "disabled" {
			inviterSub = u.ID
			break
		}
	}
	res, err := g.Pay(ctx, e.TgID, e.UserID, inviterSub)
	if err != nil {
		return err
	}
	w := growthWordsFor(b.config(ctx).Lang)
	if res.InviterD > 0 {
		b.say(inviter, fmt.Sprintf(w.bonusInviter, res.InviterD))
	}
	if res.FriendD > 0 {
		b.say(e.TgID, fmt.Sprintf(w.bonusFriend, res.FriendD))
	}
	return nil
}

// say sends a plain message to a chat. A test replaces it with a recorder.
func (b *Bot) say(chat int64, text string) {
	if b.sayHook != nil {
		b.sayHook(chat, text)
		return
	}
	b.d.Outbox.Reply(chat, "", 1, func(ctx context.Context, c *tgapi.Client) error {
		_, err := c.Send(ctx, chat, text, nil, false)
		return err
	})
}
