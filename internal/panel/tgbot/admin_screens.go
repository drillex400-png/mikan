package tgbot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"mikan/internal/panel/model"
	"mikan/internal/tgapi"
)

// The admin's screens. Callback data is "a" or "a<letter>:<args>", short and numeric. A
// screen is drawn only for an account the panel's config names an admin, and the panel checks
// that again on every call: what is shown here is never the only guard.

// adminState is what the bot waits for from one admin's next message.
type adminState struct {
	kind  string // "search" or "days"
	user  int64  // for "days": the subscription
	query string
}

type adminMem struct {
	wait   map[int64]adminState // by the admin's chat
	query  map[int64]string     // each admin's last search, for the pages
	listed map[int64]string     // the chats whose command menu lists /admin, and in which language
}

// adminAccess says whether the admin screens exist for this account and give the helpers.
func (b *Bot) adminOf(ctx context.Context, chat int64) (Admin, bool) {
	if !b.config(ctx).IsAdmin(chat) {
		return nil, false
	}
	ab, ok := b.d.Backend.(interface{ Admin() Admin })
	if !ok {
		return nil, false
	}
	return ab.Admin(), true
}

func (b *Bot) setWait(chat int64, s adminState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.adm.wait == nil {
		b.adm.wait = map[int64]adminState{}
	}
	b.adm.wait[chat] = s
}

func (b *Bot) takeWait(chat int64) (adminState, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.adm.wait[chat]
	delete(b.adm.wait, chat)
	return s, ok
}

// adminMessage handles /admin and the text an admin was asked for. It returns true when the
// message was the admin's and nothing more should be done with it.
func (b *Bot) adminMessage(ctx context.Context, m *tgapi.Message) bool {
	chat := m.Chat.ID
	text := strings.TrimSpace(m.Text)
	cmd, _, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	adm, ok := b.adminOf(ctx, chat)
	if !ok || m.Chat.Type != "private" {
		return false // not an admin: /admin is an ordinary text for the subscribers' screens
	}
	b.listAdminCommand(ctx, chat)
	cfg := b.config(ctx)
	aw := admWordsFor(cfg.Lang)
	if cmd == "/admin" {
		b.setWait(chat, adminState{})
		b.adminDraw(chat, 0, "a", "")
		return true
	}
	st, waiting := b.takeWait(chat)
	if !waiting || st.kind == "" || strings.HasPrefix(text, "/") {
		return false
	}
	switch st.kind {
	case "search":
		b.adm.setQuery(b, chat, text)
		b.adminDraw(chat, 0, "au:0", "")
	case "days":
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 1 || n > MaxAdminDays {
			b.setWait(chat, st)
			b.adminDraw(chat, 0, "au:0", fmt.Sprintf(aw.daysBad, MaxAdminDays))
			return true
		}
		b.adminDraw(chat, 0, fmt.Sprintf("ae:%d:%d", st.user, n), "")
	}
	_ = adm
	return true
}

func (a *adminMem) setQuery(b *Bot, chat int64, q string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.adm.query == nil {
		b.adm.query = map[int64]string{}
	}
	if len(q) > 64 {
		q = q[:64]
	}
	b.adm.query[chat] = q
}

func (b *Bot) queryOf(chat int64) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.adm.query[chat]
}

// adminDraw sends or edits the admin's screen. msg 0 sends a new message.
func (b *Bot) adminDraw(chat, msg int64, data, notice string) {
	key := "adm:" + strconv.FormatInt(chat, 10)
	b.d.Outbox.Reply(chat, key, 1, func(ctx context.Context, c *tgapi.Client) error {
		text, kb := b.adminScreen(ctx, chat, data, notice)
		if msg == 0 {
			_, err := c.Send(ctx, chat, text, kb, false)
			return err
		}
		return c.Edit(ctx, chat, msg, text, kb)
	})
}

// adminPress handles a tap on an admin button. It reports whether the data was the admin's.
func (b *Bot) adminPress(ctx context.Context, chat, msg int64, data string) bool {
	// Exactly the admin's codes, nothing that merely starts with "a" (the subscribers' "app").
	cmd, _, _ := strings.Cut(data, ":")
	switch cmd {
	case "a", "ao", "au", "an", "ap", "ac", "ae", "ak", "ad", "as", "az":
	default:
		return false
	}
	if _, ok := b.adminOf(ctx, chat); !ok {
		return false
	}
	b.adminDraw(chat, msg, data, "")
	return true
}

func atoi(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

// adminScreen draws one screen and, for a confirmed change, makes it first.
func (b *Bot) adminScreen(ctx context.Context, chat int64, data, notice string) (string, *tgapi.Keyboard) {
	cfg := b.config(ctx)
	aw := admWordsFor(cfg.Lang)
	adm, ok := b.adminOf(ctx, chat)
	if !ok {
		return html.EscapeString(aw.notAdmin), nil
	}
	cmd, arg, _ := strings.Cut(data, ":")
	home := []tgapi.Button{{Text: aw.back, CallbackData: "a"}}
	fail := func(err error) (string, *tgapi.Keyboard) {
		switch {
		case errors.Is(err, model.ErrNotAdmin):
			return html.EscapeString(aw.notAdmin), nil
		case errors.Is(err, model.ErrNotFound):
			return html.EscapeString(aw.notFound), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
		}
		b.d.Log.Warn("telegram: the admin's screen", "screen", cmd, "err", err)
		return html.EscapeString(aw.failed), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
	}
	pre := ""
	if notice != "" {
		pre = html.EscapeString(notice) + "\n\n"
	}
	switch cmd {
	case "ao":
		o, err := adm.Overview(ctx, chat)
		if err != nil {
			return fail(err)
		}
		t := "<b>" + html.EscapeString(aw.overview) + "</b>\n\n" + strings.Join([]string{
			fmt.Sprintf("%s: %d", aw.statUsers, o.Users), fmt.Sprintf("%s: %d", aw.statActive, o.Active),
			fmt.Sprintf("%s: %d", aw.statExpiring, o.Expiring), fmt.Sprintf("%s: %d", aw.statExpired, o.Expired),
			fmt.Sprintf("%s: %d", aw.statDisabled, o.Disabled),
			fmt.Sprintf("%s: %d/%d", aw.statNodes, o.NodesOnline, o.Nodes), fmt.Sprintf("%s: %d", aw.statPaid, o.PaidMonth),
		}, "\n")
		return pre + t, &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{{{Text: aw.refresh, CallbackData: "ao"}}, home}}
	case "au":
		page := int(atoi(arg))
		q := b.queryOf(chat)
		us, more, err := adm.Users(ctx, chat, q, page)
		if err != nil {
			return fail(err)
		}
		var rows [][]tgapi.Button
		for _, u := range us {
			rows = append(rows, []tgapi.Button{{Text: truncate(u.Name+" · "+stateMark(u.State), 40), CallbackData: "ak:" + strconv.FormatInt(u.ID, 10)}})
		}
		nav := []tgapi.Button{}
		if page > 0 {
			nav = append(nav, tgapi.Button{Text: aw.prev, CallbackData: "au:" + strconv.Itoa(page-1)})
		}
		if more {
			nav = append(nav, tgapi.Button{Text: aw.next, CallbackData: "au:" + strconv.Itoa(page+1)})
		}
		if len(nav) > 0 {
			rows = append(rows, nav)
		}
		rows = append(rows, []tgapi.Button{{Text: aw.search, CallbackData: "as"}}, home)
		t := "<b>" + html.EscapeString(aw.users) + "</b>"
		if q != "" {
			t += " · " + html.EscapeString(q)
		}
		if len(us) == 0 {
			t += "\n\n" + html.EscapeString(aw.searchNone)
		} else {
			t += "\n" + html.EscapeString(fmt.Sprintf(aw.page, page+1))
		}
		return pre + t, &tgapi.Keyboard{InlineKeyboard: rows}
	case "as":
		b.setWait(chat, adminState{kind: "search"})
		return pre + html.EscapeString(aw.searchAsk), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{{{Text: aw.back, CallbackData: "au:0"}}}}
	case "ak":
		return b.adminCard(ctx, adm, chat, atoi(arg), aw, pre, fail)
	case "ad":
		id := atoi(arg)
		b.setWait(chat, adminState{kind: "days", user: id})
		return pre + html.EscapeString(aw.daysAsk), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{{{Text: aw.cancel, CallbackData: "ak:" + strconv.FormatInt(id, 10)}}}}
	case "ae": // ae:<user>:<days> asks; az confirms
		idS, daysS, _ := strings.Cut(arg, ":")
		id, days := atoi(idS), atoi(daysS)
		u, err := adm.User(ctx, chat, id)
		if err != nil {
			return fail(err)
		}
		return pre + html.EscapeString(fmt.Sprintf(aw.askExtend, u.Name, days)), confirm(aw, "az:e:"+idS+":"+daysS, "ak:"+idS)
	case "ac": // ac:<user>:<1|0> asks to block (1) or unblock (0)
		idS, flag, _ := strings.Cut(arg, ":")
		u, err := adm.User(ctx, chat, atoi(idS))
		if err != nil {
			return fail(err)
		}
		ask := aw.askUnblock
		if flag == "1" {
			ask = aw.askBlock
		}
		return pre + html.EscapeString(fmt.Sprintf(ask, u.Name)), confirm(aw, "az:b:"+idS+":"+flag, "ak:"+idS)
	case "az": // the confirmed change
		kind, rest, _ := strings.Cut(arg, ":")
		idS, v, _ := strings.Cut(rest, ":")
		id := atoi(idS)
		var err error
		switch kind {
		case "e":
			_, err = adm.GrantDays(ctx, chat, id, atoi(v))
		case "b":
			_, err = adm.SetDisabled(ctx, chat, id, v == "1")
		default:
			return fail(model.ErrAdminInput)
		}
		if err != nil {
			return fail(err)
		}
		return b.adminCard(ctx, adm, chat, id, aw, html.EscapeString(aw.done)+"\n\n", fail)
	case "an":
		ns, err := adm.Nodes(ctx, chat)
		if err != nil {
			return fail(err)
		}
		lines := []string{"<b>" + html.EscapeString(aw.nodes) + "</b>", ""}
		for _, n := range ns {
			st := aw.nodeOff
			if n.Enabled {
				st = aw.nodeOn
			}
			lines = append(lines, html.EscapeString(n.Name)+" · "+html.EscapeString(st))
		}
		if len(ns) == 0 {
			lines = append(lines, html.EscapeString(aw.noNodes))
		}
		return pre + strings.Join(lines, "\n"), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
	case "ap":
		ps, err := adm.Payments(ctx, chat)
		if err != nil {
			return fail(err)
		}
		lines := []string{"<b>" + html.EscapeString(aw.payments) + "</b>", ""}
		for _, p := range ps {
			lines = append(lines, html.EscapeString(fmt.Sprintf("%s · %s · %.2f %s · %s", p.At.Format("02.01 15:04"), p.Status, float64(p.Amount)/100, p.Currency, p.User)))
		}
		if len(ps) == 0 {
			lines = append(lines, html.EscapeString(aw.noPayments))
		}
		return pre + strings.Join(lines, "\n"), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
	}
	// "a": the admin's first screen
	rows := [][]tgapi.Button{
		{{Text: aw.overview, CallbackData: "ao"}, {Text: aw.users, CallbackData: "au:0"}},
		{{Text: aw.nodes, CallbackData: "an"}, {Text: aw.payments, CallbackData: "ap"}},
	}
	return pre + "<b>" + html.EscapeString(aw.title) + "</b>", &tgapi.Keyboard{InlineKeyboard: rows}
}

func confirm(aw *admWords, yes, no string) *tgapi.Keyboard {
	return &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{{{Text: aw.confirm, CallbackData: yes}}, {{Text: aw.cancel, CallbackData: no}}}}
}

func (b *Bot) adminCard(ctx context.Context, adm Admin, chat, id int64, aw *admWords, pre string, fail func(error) (string, *tgapi.Keyboard)) (string, *tgapi.Keyboard) {
	u, err := adm.User(ctx, chat, id)
	if err != nil {
		return fail(err)
	}
	until := aw.noTerm
	if u.ExpiresAt > 0 {
		until = time.Unix(u.ExpiresAt, 0).Format("02.01.2006")
	}
	lim := "∞"
	if u.Limit > 0 {
		lim = gib(u.Limit)
	}
	tg := aw.noTg
	if u.TgID != 0 {
		tg = strconv.FormatInt(u.TgID, 10)
	}
	// The card names the subscription and what it has; it carries no link or token.
	t := pre + "<b>" + html.EscapeString(aw.card+" «"+u.Name+"»") + "</b>\n\n" + strings.Join([]string{
		html.EscapeString(aw.state + ": " + stateMark(u.State)), html.EscapeString(aw.until + ": " + until),
		html.EscapeString(aw.traffic + ": " + gib(u.Used) + " / " + lim), html.EscapeString(fmt.Sprintf("%s: %d", aw.devices, u.Devices)),
		html.EscapeString(aw.tg + ": " + tg),
	}, "\n")
	idS := strconv.FormatInt(id, 10)
	blk := tgapi.Button{Text: aw.block, CallbackData: "ac:" + idS + ":1"}
	if u.State == "disabled" {
		blk = tgapi.Button{Text: aw.unblock, CallbackData: "ac:" + idS + ":0"}
	}
	return t, &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{
		{{Text: aw.extend, CallbackData: "ad:" + idS}, blk},
		{{Text: aw.back, CallbackData: "au:0"}},
	}}
}

func stateMark(s string) string {
	switch s {
	case "active":
		return "🟢 active"
	case "expiring":
		return "🟡 expiring"
	case "expired":
		return "🔴 expired"
	case "limited":
		return "🟠 limited"
	case "disabled":
		return "⛔ disabled"
	}
	return s
}

func gib(b int64) string { return fmt.Sprintf("%.1f GB", float64(b)/(1<<30)) }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// listAdminCommand puts /admin in the command menu of an admin's own chat the first time the
// admin writes to the bot (and again when the bot's language changed). Every other chat's
// menu stays as it is. A failure is tried again with the next message.
func (b *Bot) listAdminCommand(ctx context.Context, chat int64) {
	cfg := b.config(ctx)
	b.mu.Lock()
	done := b.adm.listed[chat] == cfg.Lang
	b.mu.Unlock()
	if done {
		return
	}
	w, aw := wordsFor(cfg.Lang), admWordsFor(cfg.Lang)
	b.d.Go(func() {
		if err := b.d.Client.SetChatCommands(ctx, chat, w.commands, aw.cmdAdmin); err != nil {
			b.d.Log.Warn("telegram: the admin's command menu", "chat", chat, "err", err)
			return
		}
		b.mu.Lock()
		if b.adm.listed == nil {
			b.adm.listed = map[int64]string{}
		}
		b.adm.listed[chat] = cfg.Lang
		b.mu.Unlock()
	})
}
