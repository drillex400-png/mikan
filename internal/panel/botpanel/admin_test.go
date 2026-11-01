package botpanel

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/model"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgate"
	"mikan/internal/panel/tgbot"
)

const adminTg = 424242

// setAdmins saves the bot's setup with the given admins.
func setAdmins(t *testing.T, e *env, ids ...int64) {
	t.Helper()
	cfg := tgbot.Default("ru")
	cfg.AdminIDs = ids
	if err := settings.Set(e.ctx, e.set, tgate.KeyConfig, cfg); err != nil {
		t.Fatal(err)
	}
}

func auditActions(t *testing.T, e *env) []string {
	t.Helper()
	rows, err := e.st.Q.ListAudit(e.ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Action)
	}
	return out
}

// Whoever is not in the list of admins gets nothing from any admin call and changes nothing,
// whatever the bot decided to show.
func TestAdminRefusesAStranger(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	for _, by := range []int64{0, 1, -adminTg, adminTg + 1} {
		if _, err := a.Overview(ctx, by); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Overview by %d: %v", by, err)
		}
		if _, _, err := a.Users(ctx, by, "", 0); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Users by %d: %v", by, err)
		}
		if _, err := a.User(ctx, by, e.user.ID); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("User by %d: %v", by, err)
		}
		if _, err := a.GrantDays(ctx, by, e.user.ID, 30); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("GrantDays by %d: %v", by, err)
		}
		if _, err := a.SetDisabled(ctx, by, e.user.ID, true); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("SetDisabled by %d: %v", by, err)
		}
		if _, err := a.Nodes(ctx, by); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Nodes by %d: %v", by, err)
		}
		if _, err := a.Payments(ctx, by); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Payments by %d: %v", by, err)
		}
	}
	after, err := e.st.Q.GetUser(ctx, e.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ExpiresAt != e.user.ExpiresAt || after.Status != e.user.Status {
		t.Fatalf("a stranger changed the subscription: %+v -> %+v", e.user, after)
	}
	if got := auditActions(t, e); len(got) != 0 {
		t.Fatalf("a refused call was written to the log: %v", got)
	}
}

// With no admins set, nobody is one.
func TestAdminEmptyListAdmitsNobody(t *testing.T) {
	e := setup(t)
	if _, err := e.local.Admin().Overview(context.Background(), adminTg); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("no admins set: %v", err)
	}
}

// An admin extends and blocks; each change is written to the audit log with who made it, and
// carries no link or token.
func TestAdminChangesAreAudited(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()

	before := e.user.ExpiresAt.Int64
	v, err := a.GrantDays(ctx, adminTg, e.user.ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	if v.ExpiresAt <= before {
		t.Fatalf("the term did not grow: %d -> %d", before, v.ExpiresAt)
	}
	if v, err = a.SetDisabled(ctx, adminTg, e.user.ID, true); err != nil || v.State != "disabled" {
		t.Fatalf("block: %+v %v", v, err)
	}
	if v, err = a.SetDisabled(ctx, adminTg, e.user.ID, false); err != nil || v.State == "disabled" {
		t.Fatalf("unblock: %+v %v", v, err)
	}
	got := strings.Join(auditActions(t, e), ",")
	for _, want := range []string{"tgadmin.extend", "tgadmin.disable"} {
		if !strings.Contains(got, want) {
			t.Errorf("the log has no %s: %s", want, got)
		}
	}
	rows, _ := e.st.Q.ListAudit(ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 100})
	for _, r := range rows {
		if strings.HasPrefix(r.Action, "tgadmin.") {
			if !r.Details.Valid || !strings.Contains(r.Details.String, "tg:424242") {
				t.Errorf("%s does not name who made it: %v", r.Action, r.Details)
			}
			if strings.Contains(r.Details.String, e.user.SubToken) {
				t.Errorf("%s carries the subscription token", r.Action)
			}
		}
	}
}

// The bounds of one extension hold, and an unknown subscription is not found.
func TestAdminBounds(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	for _, days := range []int64{0, -5, tgbot.MaxAdminDays + 1} {
		if _, err := a.GrantDays(ctx, adminTg, e.user.ID, days); !errors.Is(err, model.ErrAdminInput) {
			t.Errorf("%d days: %v", days, err)
		}
	}
	if _, err := a.GrantDays(ctx, adminTg, e.user.ID+9999, 5); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("an unknown subscription: %v", err)
	}
	if _, err := a.User(ctx, adminTg, e.user.ID+9999); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("an unknown subscription card: %v", err)
	}
}

// A removed admin loses access on the next call, with no restart.
func TestAdminRemovedLosesAccessAtOnce(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	if _, err := a.Overview(ctx, adminTg); err != nil {
		t.Fatal(err)
	}
	setAdmins(t, e, 7)
	if _, err := a.Overview(ctx, adminTg); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("a removed admin: %v", err)
	}
}

// The overview and the list work, and the list pages.
func TestAdminReads(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	o, err := a.Overview(ctx, adminTg)
	if err != nil || o.Users < 1 {
		t.Fatalf("overview: %+v %v", o, err)
	}
	us, more, err := a.Users(ctx, adminTg, "", 0)
	if err != nil || len(us) == 0 || more {
		t.Fatalf("users: %d %v %v", len(us), more, err)
	}
	if none, _, _ := a.Users(ctx, adminTg, "no-such-name-zzz", 0); len(none) != 0 {
		t.Fatalf("a search that matches nobody found %d", len(none))
	}
	if _, err := a.Nodes(ctx, adminTg); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Payments(ctx, adminTg); err != nil {
		t.Fatal(err)
	}
}

// The whole way, as the addon takes it: Telegram, the bot, the RPC, the panel. A subscriber's
// /admin does nothing admin; an admin's opens the screens, and a change goes through a
// confirmation and lands in the log.
func TestAdminThroughTheBot(t *testing.T) {
	e := setup(t)
	const anna, boss = 555, adminTg
	setAdmins(t, e, boss)

	// a subscriber: /admin is an ordinary text, nothing of the admin's is shown
	from := e.tg.count()
	e.say(anna, "/admin")
	cs := e.tg.until(t, from, func(cs []call) bool { return len(cs) > 0 })
	time.Sleep(200 * time.Millisecond)
	for _, c := range e.tg.calls[from:] {
		if strings.Contains(text(c), "Администрирование") || strings.Contains(strings.Join(vals(buttons(c)), " "), "ao") {
			t.Fatalf("a subscriber got the admin's screen: %v", c.body)
		}
		if c.method == "setMyCommands" {
			t.Fatalf("a subscriber's command menu was touched: %v", c.body)
		}
	}
	_ = cs

	// an admin: /admin opens the first screen, and the command menu of that chat lists it
	from = e.tg.count()
	e.say(boss, "/admin")
	got := e.tg.until(t, from, func(cs []call) bool {
		_, sent := find(cs, "sendMessage")
		_, menu := find(cs, "setMyCommands")
		return sent && menu
	})
	m, _ := find(got, "sendMessage")
	if !strings.Contains(text(m), "Администрирование") {
		t.Fatalf("the admin's first screen: %q", text(m))
	}
	if b := buttons(m); b["📊 Сводка"] != "ao" || b["👥 Клиенты"] != "au:0" {
		t.Fatalf("the admin's buttons: %v", b)
	}
	cmds, _ := find(got, "setMyCommands")
	if sc, _ := cmds.body["scope"].(map[string]any); sc["chat_id"] != float64(boss) {
		t.Fatalf("the menu is not scoped to the admin's chat: %v", cmds.body)
	}

	// the card of a subscription, then a block that asks first
	const adminMsg = 9001
	uid := strconv.FormatInt(e.user.ID, 10)
	shown := func(data string) call {
		t.Helper()
		from := e.tg.count()
		e.press(boss, adminMsg, data)
		cs := e.tg.until(t, from, func(cs []call) bool { _, ok := find(cs, "editMessageText"); return ok })
		c, _ := find(cs, "editMessageText")
		return c
	}
	if c := shown("ao"); !strings.Contains(text(c), "Сводка") {
		t.Fatalf("overview: %q", text(c))
	}
	card := shown("ak:" + uid)
	if strings.Contains(text(card), "http") || strings.Contains(text(card), e.user.SubToken) {
		t.Fatalf("the card carries a link or a token: %q", text(card))
	}
	if b := buttons(card); b["⛔ Заблокировать"] != "ac:"+uid+":1" {
		t.Fatalf("the card's buttons: %v", b)
	}
	// asking changes nothing
	ask := shown("ac:" + uid + ":1")
	if b := buttons(ask); b["Да, подтверждаю"] != "az:b:"+uid+":1" {
		t.Fatalf("the confirmation: %v", b)
	}
	if v, _ := e.local.Admin().User(e.ctx, boss, e.user.ID); v.State == "disabled" {
		t.Fatalf("a question blocked the subscription: %+v", v)
	}
	// confirming does, and says who did it
	shown("az:b:" + uid + ":1")
	if v, err := e.local.Admin().User(e.ctx, boss, e.user.ID); err != nil || v.State != "disabled" {
		t.Fatalf("after the confirmation: %+v %v", v, err)
	}
	if !strings.Contains(strings.Join(auditActions(t, e), ","), "tgadmin.disable") {
		t.Fatal("the block is not in the log")
	}

	// a subscriber who knows the codes presses them: nothing of the admin's happens
	from = e.tg.count()
	e.press(anna, 9002, "az:b:"+uid+":0")
	time.Sleep(300 * time.Millisecond)
	if v, _ := e.local.Admin().User(e.ctx, boss, e.user.ID); v.State != "disabled" {
		t.Fatalf("a subscriber unblocked the subscription: %+v", v)
	}
	for _, c := range e.tg.calls[from:] {
		if strings.Contains(text(c), "Готово") || strings.Contains(text(c), "Подписка «") {
			t.Fatalf("a subscriber was shown an admin's answer: %q", text(c))
		}
	}
}

func vals(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// The admin's broadcast goes through the panel's own path and is refused to everybody else,
// whatever the bot showed. The log keeps who sent it and to how many, never the text.
func TestAdminBroadcast(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()

	var sent []string
	e.local.Broadcast = func(_ context.Context, text string) (int, error) {
		sent = append(sent, text)
		return 7, nil
	}

	// a stranger: no number, no sending, nothing queued, nothing logged
	for _, by := range []int64{0, 1, -adminTg, adminTg + 1} {
		if _, err := a.BroadcastAudience(ctx, by); !errors.Is(err, model.ErrNotAdmin) {
			t.Fatalf("audience for %d: %v", by, err)
		}
		if _, err := a.Broadcast(ctx, by, "hello"); !errors.Is(err, model.ErrNotAdmin) {
			t.Fatalf("broadcast for %d: %v", by, err)
		}
	}
	if len(sent) != 0 {
		t.Fatalf("a stranger's broadcast was sent: %q", sent)
	}

	// the text is bounded, and nothing is sent for a bad one
	for _, text := range []string{"", "   \n", strings.Repeat("я", tgbot.MaxBroadcast+1)} {
		if _, err := a.Broadcast(ctx, adminTg, text); !errors.Is(err, model.ErrAdminInput) {
			t.Fatalf("a text of %d characters: %v", len([]rune(text)), err)
		}
	}
	if len(sent) != 0 {
		t.Fatalf("a bad text was sent: %q", sent)
	}

	// the longest allowed text goes, trimmed, and the log has the count and not the text
	long := strings.Repeat("я", tgbot.MaxBroadcast)
	n, err := a.Broadcast(ctx, adminTg, "  "+long+"\n")
	if err != nil || n != 7 || len(sent) != 1 || sent[0] != long {
		t.Fatalf("the broadcast: n=%d err=%v sent=%d", n, err, len(sent))
	}
	rows, err := e.st.Q.ListAudit(e.ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 20})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.Action != "tgadmin.broadcast" {
			continue
		}
		found = true
		d := r.Details.String
		if strings.Contains(d, "яя") || !strings.Contains(d, "tg:"+strconv.Itoa(adminTg)) || !strings.Contains(d, `"recipients":7`) {
			t.Fatalf("the log entry: %s", d)
		}
	}
	if !found {
		t.Fatal("the broadcast is not in the log")
	}

	// the panel's own refusals come back as the bot's codes
	e.local.Broadcast = func(context.Context, string) (int, error) { return 0, ErrBusy }
	if _, err := a.Broadcast(ctx, adminTg, "x"); !errors.Is(err, model.ErrBroadcastBusy) {
		t.Fatalf("busy: %v", err)
	}
	e.local.Broadcast = func(context.Context, string) (int, error) { return 0, tgate.ErrOff }
	if _, err := a.Broadcast(ctx, adminTg, "x"); !errors.Is(err, model.ErrBroadcastOff) {
		t.Fatalf("off: %v", err)
	}
	e.local.Broadcast = nil
	if _, err := a.Broadcast(ctx, adminTg, "x"); !errors.Is(err, model.ErrBroadcastOff) {
		t.Fatalf("not wired: %v", err)
	}

	// an admin taken out of the list loses it at once
	setAdmins(t, e)
	e.local.Broadcast = func(context.Context, string) (int, error) { t.Fatal("sent after removal"); return 0, nil }
	if _, err := a.Broadcast(ctx, adminTg, "x"); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("after removal: %v", err)
	}
}

// The broadcast the way an admin runs it in the bot: the text, the preview with the number of
// recipients, a cancel that sends nothing, and a confirmation that sends once, to the
// subscribers only.
func TestAdminBroadcastThroughTheBot(t *testing.T) {
	e := setup(t)
	const anna, boss = 555, adminTg
	e.link(anna)
	setAdmins(t, e, boss)
	e.local.Broadcast = e.panel.Broadcast

	const adminMsg = 9100
	shown := func(who int64, data string) call {
		t.Helper()
		from := e.tg.count()
		e.press(who, adminMsg, data)
		cs := e.tg.until(t, from, func(cs []call) bool { _, ok := find(cs, "editMessageText"); return ok })
		c, _ := find(cs, "editMessageText")
		return c
	}
	sentTo := func(from int, chat int64) int {
		time.Sleep(300 * time.Millisecond)
		e.tg.mu.Lock()
		defer e.tg.mu.Unlock()
		n := 0
		for _, c := range e.tg.calls[from:] {
			if c.method == "sendMessage" && c.body["chat_id"] == float64(chat) && strings.Contains(text(c), "Плановые работы") {
				n++
			}
		}
		return n
	}

	// the first screen offers it; asking waits for the text
	home := shown(boss, "a")
	if buttons(home)["📣 Рассылка"] != "ab" {
		t.Fatalf("the first screen: %v", buttons(home))
	}
	if c := shown(boss, "ab"); !strings.Contains(text(c), "текст рассылки") {
		t.Fatalf("the ask: %q", text(c))
	}

	// the text comes as an ordinary message: the preview shows it as the clients get it, with the
	// number of recipients, and sends nothing yet
	from := e.tg.count()
	e.say(boss, "Плановые работы в {brand} ночью")
	cs := e.tg.until(t, from, func(cs []call) bool {
		for _, c := range cs {
			if c.method == "sendMessage" && strings.Contains(text(c), "увидят клиенты") {
				return true
			}
		}
		return false
	})
	var preview call
	for _, c := range cs {
		if strings.Contains(text(c), "увидят клиенты") {
			preview = c
		}
	}
	if !strings.Contains(text(preview), "Плановые работы в Mikan VPN ночью") || !strings.Contains(text(preview), "Получателей: 1") {
		t.Fatalf("the preview: %q", text(preview))
	}
	if b := buttons(preview); b["Да, отправить"] != "az:m" || b["Отмена"] != "ab:x" {
		t.Fatalf("the preview's buttons: %v", b)
	}
	if sentTo(from, anna) != 0 {
		t.Fatal("the preview sent the broadcast")
	}

	// a cancel sends nothing and forgets the text: a late confirmation finds nothing to send
	shown(boss, "ab:x")
	from = e.tg.count()
	if c := shown(boss, "az:m"); !strings.Contains(text(c), "потерялся") {
		t.Fatalf("a confirmation without a text: %q", text(c))
	}
	if sentTo(from, anna) != 0 {
		t.Fatal("a cancelled broadcast was sent")
	}

	// a subscriber who knows the codes sends nothing and is shown nothing of the admin's
	from = e.tg.count()
	e.press(anna, 9101, "az:m")
	e.press(anna, 9101, "ab:p")
	time.Sleep(300 * time.Millisecond)
	for _, c := range e.tg.calls[from:] {
		if strings.Contains(text(c), "Рассылка") || strings.Contains(text(c), "получател") {
			t.Fatalf("a subscriber was shown the admin's broadcast: %q", text(c))
		}
	}

	// the confirmed one goes once, to the subscriber, and the admin is told how many
	e.say(boss, "/admin")
	shown(boss, "ab")
	e.say(boss, "Плановые работы в {brand} ночью")
	e.tg.until(t, e.tg.count()-1, func([]call) bool { return true })
	time.Sleep(300 * time.Millisecond)
	from = e.tg.count()
	done := shown(boss, "az:m")
	if !strings.Contains(text(done), "Получателей: 1") {
		t.Fatalf("the report: %q", text(done))
	}
	if got := sentTo(from, anna); got != 1 {
		t.Fatalf("the subscriber got the broadcast %d times", got)
	}
	// pressing the confirmation again finds no text: it cannot send twice
	from = e.tg.count()
	if c := shown(boss, "az:m"); !strings.Contains(text(c), "потерялся") {
		t.Fatalf("a second confirmation: %q", text(c))
	}
	if sentTo(from, anna) != 0 {
		t.Fatal("the broadcast was sent twice")
	}
	if !strings.Contains(strings.Join(auditActions(t, e), ","), "tgadmin.broadcast") {
		t.Fatal("the broadcast is not in the log")
	}
}
