package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mikan/internal/panel/events"
	"mikan/internal/tgapi"
)

// What the admins are told as it happens: a payment, a refund, a new subscription, a server
// that went down or came back. They go to every account in Config.AdminIDs, in the bot, with
// no setting of their own: an admin who does not want them is taken out of the list.
//
// The admin's notice is separate from the subscriber's: the same event may also thank the
// buyer (HandleEvent), and one never stands in for the other. A notice carries no link, no
// token and no address of a node: an event holds none of them, and none is added here.

// AdminEventTypes are the events the admins are told of.
var AdminEventTypes = []string{events.PaymentPaid, events.PaymentRefunded, events.UserCreated, events.NodeDown, events.NodeUp}

// isAdminEvent says whether an event of this type is one the admins are told of.
func isAdminEvent(t string) bool {
	for _, x := range AdminEventTypes {
		if x == t {
			return true
		}
	}
	return false
}

// adminNotice is the text of an event for the admins, or "" when it has nothing to tell. name
// is the subscription's name when it is known. The text is HTML for Telegram: all data in it is
// escaped.
func adminNotice(aw *admWords, e events.Event, name string, now time.Time) string {
	who := func() string {
		switch {
		case name != "":
			return "\n" + kv(aw.payUser, name)
		case e.TgID != 0:
			return "\n" + kv(aw.tg, fmt.Sprint(e.TgID))
		}
		return ""
	}
	switch e.Type {
	case events.PaymentPaid, events.PaymentRefunded:
		var p events.Payment
		if json.Unmarshal(e.Data, &p) != nil {
			return ""
		}
		icon, title := "💰", aw.ntPaid
		if e.Type == events.PaymentRefunded {
			icon, title = "↩️", aw.ntRefund
		}
		t := head(icon, title) + "\n" + kv(aw.payAmount, money(p.Amount, p.Currency)) + "\n" + kv(aw.payKind, kindWord(aw, p.Kind))
		if p.TariffName != "" {
			t += "\n" + kv(aw.payTariff, p.TariffName)
		}
		if p.TermDays > 0 {
			t += "\n" + kv(aw.payTerm, fmt.Sprint(p.TermDays))
		}
		return t + who() + "\n" + kv(aw.payBy, p.Provider)
	case events.UserCreated:
		var u events.User
		if json.Unmarshal(e.Data, &u) != nil {
			return ""
		}
		return head("🆕", aw.ntNewUser) + who() + "\n" + kv(aw.ntSource, sourceWord(aw, u.Source))
	case events.NodeDown, events.NodeUp:
		var n events.Node
		if json.Unmarshal(e.Data, &n) != nil || n.Name == "" {
			return ""
		}
		if e.Type == events.NodeUp {
			t := head("🟢", aw.ntNodeUp) + "\n" + kv(aw.srvTitle, n.Name)
			if n.DownAt > 0 {
				t += "\n" + kv(aw.ntDownFor, since(aw, time.Unix(n.DownAt, 0), now))
			}
			return t
		}
		return head("🔴", aw.ntNodeDown) + "\n" + kv(aw.srvTitle, n.Name)
	}
	return ""
}

// sourceWord names how a subscription came to be.
func sourceWord(aw *admWords, s string) string {
	switch s {
	case "admin":
		return aw.srcAdmin
	case "bot":
		return aw.srcBot
	case "trial":
		return aw.kindTrial
	case "free":
		return aw.srcFree
	}
	return s
}

// since is how long ago at was, in minutes, hours or days.
func since(aw *admWords, at, now time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return aw.durSecs
	case d < time.Hour:
		return fmt.Sprintf(aw.durMins, int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf(aw.durHours, int(d.Hours()))
	}
	return fmt.Sprintf(aw.durDays, int(d.Hours()/24))
}

// tellAdmins sends the notice of an event to every admin. It does not fail the event: an admin
// who blocked the bot must not make the event come again for the subscribers.
func (b *Bot) tellAdmins(ctx context.Context, e events.Event) {
	if !isAdminEvent(e.Type) {
		return
	}
	cfg := b.config(ctx)
	if len(cfg.AdminIDs) == 0 {
		return
	}
	name := ""
	if e.UserID != 0 && e.Type != events.PaymentRefunded {
		if u, err := b.d.Backend.User(ctx, e.UserID); err == nil {
			name = u.Name
		}
	}
	text := adminNotice(admWordsFor(cfg.Lang), e, name, b.d.Now())
	if text == "" {
		return
	}
	for _, id := range cfg.AdminIDs {
		chat := id
		b.d.Outbox.Notice(chat, func(ctx context.Context, c *tgapi.Client) error {
			_, err := c.Send(ctx, chat, text, nil, false)
			return err
		}, nil)
	}
}
