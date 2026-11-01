package tgbot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mikan/internal/panel/model"
	"mikan/internal/tgapi"
)

// The admin's screens. Callback data is "a" or "a<letter>:<args>", short and numeric. A
// screen is drawn only for an account the panel's config names an admin, and the panel checks
// that again on every call: what is shown here is never the only guard.

// adminState is what the bot waits for from one admin's next message.
type adminState struct {
	kind  string // search, days, broadcast, gb, devices or note
	user  int64  // for days, gb, devices and note: the subscription
	query string
}

type adminMem struct {
	wait   map[int64]adminState // by the admin's chat
	query  map[int64]string     // each admin's last search, for the pages
	listed map[int64]string     // the chats whose command menu lists /admin, and in which language
	draft  map[int64]string     // each admin's broadcast text, held until it is confirmed
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

func (b *Bot) setDraft(chat int64, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.adm.draft == nil {
		b.adm.draft = map[int64]string{}
	}
	b.adm.draft[chat] = text
}

func (b *Bot) draftOf(chat int64) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, ok := b.adm.draft[chat]
	return t, ok
}

func (b *Bot) dropDraft(chat int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.adm.draft, chat)
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
	if b.srvText(chat, st, text, aw, srvWordsFor(cfg.Lang)) {
		return true
	}
	switch st.kind {
	case "search":
		b.adm.setQuery(b, chat, text)
		b.adminDraw(chat, 0, "au:0", "")
	case "broadcast":
		if n := utf8.RuneCountInString(text); n == 0 || n > MaxBroadcast {
			b.setWait(chat, st)
			b.adminDraw(chat, 0, "ab", fmt.Sprintf(aw.bcBad, MaxBroadcast))
			return true
		}
		b.setDraft(chat, text)
		b.adminDraw(chat, 0, "ab:p", "")
	case "days":
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 1 || n > MaxAdminDays {
			b.setWait(chat, st)
			b.adminDraw(chat, 0, "au:0", fmt.Sprintf(aw.daysBad, MaxAdminDays))
			return true
		}
		b.adminDraw(chat, 0, fmt.Sprintf("ae:%d:%d", st.user, n), "")
	case "gb":
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 0 || n > MaxLimitGB {
			b.setWait(chat, st)
			b.adminDraw(chat, 0, fmt.Sprintf("ak:%d", st.user), fmt.Sprintf(aw.gbBad, MaxLimitGB))
			return true
		}
		b.adminDraw(chat, 0, fmt.Sprintf("ag:%d:%d", st.user, n), "")
	case "devices":
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 0 || n > MaxDevices {
			b.setWait(chat, st)
			b.adminDraw(chat, 0, fmt.Sprintf("ak:%d", st.user), fmt.Sprintf(aw.devBad, MaxDevices))
			return true
		}
		b.adminDraw(chat, 0, fmt.Sprintf("av:%d:%d", st.user, n), "")
	case "note":
		if utf8.RuneCountInString(text) > MaxNote {
			b.setWait(chat, st)
			b.adminDraw(chat, 0, fmt.Sprintf("ak:%d", st.user), fmt.Sprintf(aw.noteBad, MaxNote))
			return true
		}
		if text == "-" {
			text = ""
		}
		b.setDraft(chat, text)
		b.adminDraw(chat, 0, fmt.Sprintf("aN:%d:s", st.user), "")
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
	case "a", "ao", "au", "an", "ap", "aP", "ac", "ae", "ak", "ad", "as", "az", "ab",
		"ag", "av", "ar", "ai", "ax", "at", "aT", "aN", "aS":
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
	kb := func(rows ...[]tgapi.Button) *tgapi.Keyboard { return &tgapi.Keyboard{InlineKeyboard: rows} }
	fail := func(err error) (string, *tgapi.Keyboard) {
		switch {
		case errors.Is(err, model.ErrNotAdmin):
			return html.EscapeString(aw.notAdmin), nil
		case errors.Is(err, model.ErrNotFound):
			return html.EscapeString(aw.notFound), kb(home)
		}
		b.d.Log.Warn("telegram: the admin's screen", "screen", cmd, "err", err)
		return html.EscapeString(failText(aw, err)), kb(home)
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
		t := head("📊", aw.overview) + "\n\n" + strings.Join([]string{
			pill("👥", aw.statUsers, o.Users),
			pill("🟢", aw.statActive, o.Active),
			pill("🟡", aw.statExpiring, o.Expiring),
			pill("🔴", aw.statExpired, o.Expired),
			pill("⛔", aw.statDisabled, o.Disabled),
			"",
			html.EscapeString("🖥 "+aw.statNodes+": ") + fmt.Sprintf("<b>%d/%d</b>", o.NodesOnline, o.Nodes),
			pill("💳", aw.statPaid, o.PaidMonth),
		}, "\n") + overviewExtra(aw, o)
		return pre + t, kb([]tgapi.Button{{Text: aw.refresh, CallbackData: "ao"}, {Text: aw.fSoon + " (" + strconv.FormatInt(o.Expiring, 10) + ")", CallbackData: "au:s:0"}}, home)
	case "au": // au:<page> (all) or au:<filter>:<page>
		fl, pg := "", arg
		if f, p, ok := strings.Cut(arg, ":"); ok {
			fl, pg = f, p
		}
		page := int(atoi(pg))
		q := b.queryOf(chat)
		us, more, err := adm.UsersBy(ctx, chat, fl, q, page)
		if errors.Is(err, model.ErrAdminInput) && fl == "" || failKind(err) == failOld && fl == "" {
			// A panel older than the bot has no filters: the plain list is what it serves.
			us, more, err = adm.Users(ctx, chat, q, page)
		}
		if err != nil {
			return fail(err)
		}
		var rows [][]tgapi.Button
		for _, u := range us {
			rows = append(rows, []tgapi.Button{{Text: truncate(stateDot(u.State)+" "+u.Name, 40), CallbackData: "ak:" + strconv.FormatInt(u.ID, 10)}})
		}
		nav := []tgapi.Button{}
		if page > 0 {
			nav = append(nav, tgapi.Button{Text: aw.prev, CallbackData: "au:" + fl + ":" + strconv.Itoa(page-1)})
		}
		if more {
			nav = append(nav, tgapi.Button{Text: aw.next, CallbackData: "au:" + fl + ":" + strconv.Itoa(page+1)})
		}
		if len(nav) > 0 {
			rows = append(rows, nav)
		}
		rows = append(rows,
			[]tgapi.Button{{Text: filterMark(fl, "", aw.fAll), CallbackData: "au::0"}, {Text: filterMark(fl, "s", aw.fSoon), CallbackData: "au:s:0"}, {Text: filterMark(fl, "e", aw.fEnded), CallbackData: "au:e:0"}},
			[]tgapi.Button{{Text: filterMark(fl, "t", aw.fLow), CallbackData: "au:t:0"}, {Text: filterMark(fl, "n", aw.fNoTG), CallbackData: "au:n:0"}, {Text: filterMark(fl, "b", aw.fBlocked), CallbackData: "au:b:0"}},
			[]tgapi.Button{{Text: aw.search, CallbackData: "as"}}, home)
		t := head("👥", aw.users)
		if fl != "" {
			t += "\n" + html.EscapeString(aw.fShown+": "+userFilterName(aw, fl))
		}
		if q != "" {
			t += "\n🔎 " + html.EscapeString(q)
		}
		if len(us) == 0 {
			msg := aw.searchNone
			if fl != "" {
				msg = aw.fNone
			}
			t += "\n\n" + html.EscapeString(msg)
		} else {
			t += "\n" + html.EscapeString(fmt.Sprintf(aw.page, page+1))
		}
		return pre + t, kb(rows...)
	case "as":
		b.setWait(chat, adminState{kind: "search"})
		return pre + html.EscapeString(aw.searchAsk), kb([]tgapi.Button{{Text: aw.back, CallbackData: "au:0"}})
	case "ak":
		return b.adminCard(ctx, adm, chat, atoi(arg), aw, pre, fail)
	case "ad":
		id := atoi(arg)
		b.setWait(chat, adminState{kind: "days", user: id})
		idS := strconv.FormatInt(id, 10)
		return pre + html.EscapeString(aw.daysAsk), kb(
			[]tgapi.Button{{Text: "+7", CallbackData: "ae:" + idS + ":7"}, {Text: "+30", CallbackData: "ae:" + idS + ":30"},
				{Text: "+90", CallbackData: "ae:" + idS + ":90"}, {Text: "+365", CallbackData: "ae:" + idS + ":365"}},
			[]tgapi.Button{{Text: aw.cancel, CallbackData: "ak:" + idS}})
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
	case "ag": // ag:<user> asks for the GB; ag:<user>:<gb> asks to confirm it
		idS, gbS, has := strings.Cut(arg, ":")
		id := atoi(idS)
		if !has {
			b.setWait(chat, adminState{kind: "gb", user: id})
			return pre + html.EscapeString(fmt.Sprintf(aw.gbAsk, MaxLimitGB)), kb(
				[]tgapi.Button{{Text: "50", CallbackData: "ag:" + idS + ":50"}, {Text: "100", CallbackData: "ag:" + idS + ":100"},
					{Text: "300", CallbackData: "ag:" + idS + ":300"}, {Text: "∞", CallbackData: "ag:" + idS + ":0"}},
				[]tgapi.Button{{Text: aw.cancel, CallbackData: "ak:" + idS}})
		}
		u, err := adm.User(ctx, chat, id)
		if err != nil {
			return fail(err)
		}
		return pre + html.EscapeString(fmt.Sprintf(aw.askGB, u.Name, limitWord(aw, atoi(gbS)))), confirm(aw, "az:g:"+idS+":"+gbS, "ak:"+idS)
	case "av":
		idS, nS, has := strings.Cut(arg, ":")
		id := atoi(idS)
		if !has {
			b.setWait(chat, adminState{kind: "devices", user: id})
			return pre + html.EscapeString(fmt.Sprintf(aw.devAsk, MaxDevices)), kb(
				[]tgapi.Button{{Text: "1", CallbackData: "av:" + idS + ":1"}, {Text: "3", CallbackData: "av:" + idS + ":3"},
					{Text: "5", CallbackData: "av:" + idS + ":5"}, {Text: "∞", CallbackData: "av:" + idS + ":0"}},
				[]tgapi.Button{{Text: aw.cancel, CallbackData: "ak:" + idS}})
		}
		u, err := adm.User(ctx, chat, id)
		if err != nil {
			return fail(err)
		}
		return pre + html.EscapeString(fmt.Sprintf(aw.askDev, u.Name, limitWord(aw, atoi(nS)))), confirm(aw, "az:v:"+idS+":"+nS, "ak:"+idS)
	case "ar":
		u, err := adm.User(ctx, chat, atoi(arg))
		if err != nil {
			return fail(err)
		}
		return pre + html.EscapeString(fmt.Sprintf(aw.askReset, u.Name)), confirm(aw, "az:r:"+arg, "ak:"+arg)
	case "ai":
		u, err := adm.User(ctx, chat, atoi(arg))
		if err != nil {
			return fail(err)
		}
		return pre + html.EscapeString(fmt.Sprintf(aw.askReissue, u.Name)), confirm(aw, "az:i:"+arg, "ak:"+arg)
	case "ax": // ax:<user> asks; ax:<user>:2 asks again; the second yes deletes
		idS, step, _ := strings.Cut(arg, ":")
		u, err := adm.User(ctx, chat, atoi(idS))
		if err != nil {
			return fail(err)
		}
		if step == "2" {
			return pre + html.EscapeString(fmt.Sprintf(aw.askDelete2, u.Name)), confirm(aw, "az:x:"+idS, "ak:"+idS)
		}
		return pre + html.EscapeString(fmt.Sprintf(aw.askDelete, u.Name)), confirm(aw, "ax:"+idS+":2", "ak:"+idS)
	case "aN": // aN:<user> asks for the note; aN:<user>:s asks to confirm the held one
		idS, step, _ := strings.Cut(arg, ":")
		id := atoi(idS)
		if step != "s" {
			b.dropDraft(chat)
			b.setWait(chat, adminState{kind: "note", user: id})
			return pre + html.EscapeString(fmt.Sprintf(aw.noteAsk, MaxNote)), kb([]tgapi.Button{{Text: aw.cancel, CallbackData: "ak:" + idS}})
		}
		text, ok := b.draftOf(chat)
		if !ok {
			return b.adminCard(ctx, adm, chat, id, aw, pre, fail)
		}
		if _, err := adm.SetNote(ctx, chat, id, text); err != nil {
			return fail(err)
		}
		b.dropDraft(chat)
		return b.adminCard(ctx, adm, chat, id, aw, html.EscapeString(aw.done)+"\n\n", fail)
	case "at": // at:<user> lists the plans; aT:<user>:<plan> asks to apply one
		ts, err := adm.Tariffs(ctx, chat)
		if err != nil {
			return fail(err)
		}
		var rows [][]tgapi.Button
		for _, t := range ts {
			rows = append(rows, []tgapi.Button{{Text: truncate("🏷 "+t.Name+" · "+fmt.Sprintf("%d д", t.Days), 40), CallbackData: fmt.Sprintf("aT:%s:%d", arg, t.ID)}})
		}
		rows = append(rows, []tgapi.Button{{Text: aw.back, CallbackData: "ak:" + arg}})
		t := head("🏷", aw.tariffBtn)
		if len(ts) == 0 {
			t += "\n\n" + html.EscapeString(aw.noTariffs)
		}
		return pre + t, kb(rows...)
	case "aT":
		idS, tS, _ := strings.Cut(arg, ":")
		u, err := adm.User(ctx, chat, atoi(idS))
		if err != nil {
			return fail(err)
		}
		ts, err := adm.Tariffs(ctx, chat)
		if err != nil {
			return fail(err)
		}
		name := ""
		for _, t := range ts {
			if t.ID == atoi(tS) {
				name = t.Name
			}
		}
		if name == "" {
			return fail(model.ErrNotFound)
		}
		return pre + html.EscapeString(fmt.Sprintf(aw.askTariff, name, u.Name)), confirm(aw, "az:t:"+idS+":"+tS, "ak:"+idS)
	case "az": // the confirmed change
		if arg == "m" { // the broadcast: its text is the one held for this admin, once
			text, ok := b.draftOf(chat)
			b.dropDraft(chat)
			if !ok {
				return pre + html.EscapeString(aw.bcGone), kb([]tgapi.Button{{Text: aw.broadcast, CallbackData: "ab"}}, home)
			}
			n, err := adm.Broadcast(ctx, chat, text)
			switch {
			case errors.Is(err, model.ErrBroadcastBusy):
				return pre + html.EscapeString(aw.bcBusy), kb(home)
			case errors.Is(err, model.ErrBroadcastOff):
				return pre + html.EscapeString(aw.bcOff), kb(home)
			case err != nil:
				return fail(err)
			}
			return pre + html.EscapeString(fmt.Sprintf(aw.bcSent, n)), kb(home)
		}
		kind, rest, _ := strings.Cut(arg, ":")
		idS, v, _ := strings.Cut(rest, ":")
		id := atoi(idS)
		done := html.EscapeString(aw.done) + "\n\n"
		var err error
		switch kind {
		case "e":
			_, err = adm.GrantDays(ctx, chat, id, atoi(v))
		case "b":
			_, err = adm.SetDisabled(ctx, chat, id, v == "1")
		case "g":
			_, err = adm.SetTrafficLimit(ctx, chat, id, atoi(v))
		case "v":
			_, err = adm.SetDeviceLimit(ctx, chat, id, atoi(v))
		case "r":
			_, err = adm.ResetTraffic(ctx, chat, id)
		case "t":
			_, err = adm.SetTariff(ctx, chat, id, atoi(v))
		case "i":
			_, err = adm.Reissue(ctx, chat, id)
			done = html.EscapeString(aw.reissued) + "\n\n"
		case "x":
			if err = adm.Delete(ctx, chat, id); err == nil {
				return html.EscapeString(aw.deleted), kb([]tgapi.Button{{Text: aw.users, CallbackData: "au:0"}}, home)
			}
		default:
			return fail(model.ErrAdminInput)
		}
		if err != nil {
			return fail(err)
		}
		return b.adminCard(ctx, adm, chat, id, aw, done, fail)
	case "ab": // ab asks for the text; ab:p previews it and asks to confirm
		if arg == "x" {
			b.dropDraft(chat)
			return pre + "<b>" + html.EscapeString(aw.title) + "</b>", b.adminHome(aw)
		}
		if arg != "p" {
			b.dropDraft(chat)
			b.setWait(chat, adminState{kind: "broadcast"})
			return pre + html.EscapeString(fmt.Sprintf(aw.bcAsk, MaxBroadcast)), kb([]tgapi.Button{{Text: aw.cancel, CallbackData: "a"}})
		}
		text, ok := b.draftOf(chat)
		if !ok {
			return pre + html.EscapeString(aw.bcGone), kb([]tgapi.Button{{Text: aw.broadcast, CallbackData: "ab"}}, home)
		}
		n, err := adm.BroadcastAudience(ctx, chat)
		if err != nil {
			return fail(err)
		}
		if n == 0 {
			b.dropDraft(chat)
			return pre + html.EscapeString(aw.bcNone), kb(home)
		}
		// The text is shown as the clients get it: the same rendering, the same variables.
		body := Render(text, map[string]string{"brand": BrandOf(ctx, b.d.Backend)})
		return pre + html.EscapeString(aw.bcPreview) + "\n\n" + body + "\n\n" + html.EscapeString(fmt.Sprintf(aw.bcAsk2, n)),
			kb([]tgapi.Button{{Text: aw.bcSend, CallbackData: "az:m"}}, []tgapi.Button{{Text: aw.cancel, CallbackData: "ab:x"}})
	case "an":
		return b.srvList(ctx, adm, chat, aw, srvWordsFor(cfg.Lang), pre, fail, home)
	case "aS":
		return b.srvScreen(ctx, adm, chat, arg, aw, srvWordsFor(cfg.Lang), pre, fail, home)
	case "ap": // ap:<filter>:<page>; the filter is a letter, "" all
		fl, pgS, _ := strings.Cut(arg, ":")
		status := payFilterStatus(fl)
		page := int(atoi(pgS))
		ps, more, err := adm.Payments(ctx, chat, status, page)
		if err != nil {
			return fail(err)
		}
		var rows [][]tgapi.Button
		for _, p := range ps {
			rows = append(rows, []tgapi.Button{{Text: truncate(fmt.Sprintf("%s %s · %s · %s", payDot(p.Status), p.At.Format("02.01 15:04"), money(p.Amount, p.Currency), p.User), 44),
				CallbackData: "aP:" + strconv.FormatInt(p.ID, 10) + ":" + fl + ":" + strconv.Itoa(page)}})
		}
		nav := []tgapi.Button{}
		if page > 0 {
			nav = append(nav, tgapi.Button{Text: aw.prev, CallbackData: "ap:" + fl + ":" + strconv.Itoa(page-1)})
		}
		if more {
			nav = append(nav, tgapi.Button{Text: aw.next, CallbackData: "ap:" + fl + ":" + strconv.Itoa(page+1)})
		}
		if len(nav) > 0 {
			rows = append(rows, nav)
		}
		rows = append(rows,
			[]tgapi.Button{{Text: filterMark(fl, "", aw.payAll), CallbackData: "ap::0"}, {Text: filterMark(fl, "p", aw.payPaid), CallbackData: "ap:p:0"}},
			[]tgapi.Button{{Text: filterMark(fl, "w", aw.payPending), CallbackData: "ap:w:0"}, {Text: filterMark(fl, "f", aw.payFailed), CallbackData: "ap:f:0"},
				{Text: filterMark(fl, "r", aw.payRefunded), CallbackData: "ap:r:0"}},
			home)
		t := head("💳", aw.payments)
		if len(ps) == 0 {
			t += "\n\n" + html.EscapeString(aw.noPayments)
		} else {
			t += "\n" + html.EscapeString(fmt.Sprintf(aw.page, page+1))
		}
		return pre + t, kb(rows...)
	case "aP": // aP:<id>:<filter>:<page>
		parts := strings.SplitN(arg, ":", 3)
		for len(parts) < 3 {
			parts = append(parts, "")
		}
		p, err := adm.Payment(ctx, chat, atoi(parts[0]))
		if err != nil {
			return fail(err)
		}
		lines := []string{head("💳", fmt.Sprintf("%s #%d", aw.payCard, p.ID)), "",
			html.EscapeString(payLabel(aw, p.Status)),
			kv(aw.payKind, kindWord(aw, p.Kind)),
			kv(aw.payAmount, money(p.Amount, p.Currency)),
		}
		if p.Tariff != "" {
			lines = append(lines, kv(aw.payTariff, p.Tariff))
		}
		if p.TermDays > 0 {
			lines = append(lines, kv(aw.payTerm, fmt.Sprintf("%d", p.TermDays)))
		}
		if p.User != "" {
			lines = append(lines, kv(aw.payUser, p.User))
		}
		lines = append(lines, kv(aw.payBy, p.Provider), kv("🕒", p.At.Format("02.01.2006 15:04")))
		if p.PaidAt > 0 {
			lines = append(lines, kv(aw.payPaidAt, time.Unix(p.PaidAt, 0).Format("02.01.2006 15:04")))
		}
		if p.Error != "" {
			lines = append(lines, kv(aw.payErr, truncate(p.Error, 200)))
		}
		var rows [][]tgapi.Button
		if p.UserID != 0 {
			rows = append(rows, []tgapi.Button{{Text: aw.payOpenUser, CallbackData: "ak:" + strconv.FormatInt(p.UserID, 10)}})
		}
		rows = append(rows, []tgapi.Button{{Text: aw.back, CallbackData: "ap:" + parts[1] + ":" + parts[2]}})
		return pre + strings.Join(lines, "\n"), kb(rows...)
	}
	// "a": the admin's first screen
	return pre + "<b>" + html.EscapeString(aw.title) + "</b>", b.adminHome(aw)
}

// payFilterStatus maps the filter letter in a payment list's callback to the status it asks.
func payFilterStatus(f string) string {
	switch f {
	case "p":
		return "applied"
	case "w":
		return "pending"
	case "f":
		return "failed"
	case "r":
		return "refunded"
	}
	return ""
}

// filterMark puts a dot before the filter that is on.
func filterMark(cur, this, label string) string {
	if cur == this {
		return "● " + label
	}
	return label
}

func kindWord(aw *admWords, k string) string {
	switch k {
	case "new":
		return aw.kindNew
	case "renew":
		return aw.kindRenew
	case "package":
		return aw.kindPackage
	case "trial":
		return aw.kindTrial
	case "device":
		return aw.kindDev
	}
	return k
}

// limitWord is a limit as a number, or the word for none.
func limitWord(aw *admWords, n int64) string {
	if n <= 0 {
		return aw.unlimited
	}
	return strconv.FormatInt(n, 10)
}

// adminHome is the keyboard of the admin's first screen.
func (b *Bot) adminHome(aw *admWords) *tgapi.Keyboard {
	return &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{
		{{Text: aw.overview, CallbackData: "ao"}, {Text: aw.users, CallbackData: "au:0"}},
		{{Text: aw.nodes, CallbackData: "an"}, {Text: aw.payments, CallbackData: "ap::0"}},
		{{Text: aw.broadcast, CallbackData: "ab"}},
	}}
}

func confirm(aw *admWords, yes, no string) *tgapi.Keyboard {
	return &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{{{Text: aw.confirm, CallbackData: yes}, {Text: aw.cancel, CallbackData: no}}}}
}

func (b *Bot) adminCard(ctx context.Context, adm Admin, chat, id int64, aw *admWords, pre string, fail func(error) (string, *tgapi.Keyboard)) (string, *tgapi.Keyboard) {
	u, err := adm.User(ctx, chat, id)
	if err != nil {
		return fail(err)
	}
	now := b.d.Now()
	until := aw.noTerm
	if u.ExpiresAt > 0 {
		at := time.Unix(u.ExpiresAt, 0)
		until = at.Format("02.01.2006") + " · " + left(aw, at, now)
	}
	devices := fmt.Sprintf("%d", u.Devices)
	if u.DeviceLimit > 0 {
		devices = fmt.Sprintf("%d / %d", u.Devices, u.DeviceLimit)
	}
	tg := aw.noTg
	if u.TgID != 0 {
		tg = strconv.FormatInt(u.TgID, 10)
	}
	tariff := aw.noTariff
	if u.Tariff != "" {
		tariff = u.Tariff
	}
	lim := aw.unlimited
	if u.Limit > 0 {
		lim = gib(u.Limit)
	}
	// The card names the subscription and what it has; it carries no link or token.
	lines := []string{
		head("👤", u.Name), "",
		html.EscapeString(stateLabel(aw, u.State)), "",
		kv(aw.until, until),
		kv(aw.cardTariff, tariff),
		html.EscapeString(aw.traffic+": ") + "<b>" + html.EscapeString(gib(u.Used)+" / "+lim) + "</b>",
		"<code>" + bar(u.Used, u.Limit) + "</code>",
		kv(aw.devices, devices),
		kv(aw.tg, tg),
	}
	if u.CreatedAt > 0 {
		lines = append(lines, kv(aw.cardSince, time.Unix(u.CreatedAt, 0).Format("02.01.2006")))
	}
	if u.Contact != "" {
		lines = append(lines, kv("✉️", u.Contact))
	}
	if u.Note != "" {
		lines = append(lines, "", html.EscapeString("📝 "+aw.cardNote+": ")+html.EscapeString(truncate(u.Note, 300)))
	}
	idS := strconv.FormatInt(id, 10)
	blk := tgapi.Button{Text: aw.block, CallbackData: "ac:" + idS + ":1"}
	if u.State == "disabled" {
		blk = tgapi.Button{Text: aw.unblock, CallbackData: "ac:" + idS + ":0"}
	}
	return pre + strings.Join(lines, "\n"), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{
		{{Text: "+7", CallbackData: "ae:" + idS + ":7"}, {Text: "+30", CallbackData: "ae:" + idS + ":30"}, {Text: "+90", CallbackData: "ae:" + idS + ":90"},
			{Text: aw.other, CallbackData: "ad:" + idS}},
		{{Text: aw.traffLimit, CallbackData: "ag:" + idS}, {Text: aw.devLimit, CallbackData: "av:" + idS}},
		{{Text: aw.resetTr, CallbackData: "ar:" + idS}, {Text: aw.reissue, CallbackData: "ai:" + idS}},
		{{Text: aw.tariffBtn, CallbackData: "at:" + idS}, {Text: aw.noteBtn, CallbackData: "aN:" + idS}},
		{blk, {Text: aw.delBtn, CallbackData: "ax:" + idS}},
		{{Text: aw.back, CallbackData: "au:0"}},
	}}
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
