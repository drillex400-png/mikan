package tgbot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/model"
	"mikan/internal/tgapi"
)

const boss int64 = 900

// fakeAdmin records what the screens asked of the panel.
type fakeAdmin struct {
	calls []string
	user  AdminUser
	// by is the overview's extra figures; usersByErr is what UsersBy answers with, and
	// filters what it was asked for.
	over       AdminOverview
	usersByErr error
	filters    []string
	none       bool // the filter finds nobody
	// nodeErr is what the server actions answer with; node is the server's card.
	nodeErr error
	node    AdminNodeInfo
}

func (f *fakeAdmin) rec(s string) { f.calls = append(f.calls, s) }

func (f *fakeAdmin) Overview(context.Context, int64) (AdminOverview, error) {
	o := AdminOverview{Users: 10, Active: 6, Expiring: 2, Expired: 1, Disabled: 1, Nodes: 2, NodesOnline: 1, PaidMonth: 4}
	o.NewToday, o.Revenue = f.over.NewToday, f.over.Revenue
	return o, nil
}
func (f *fakeAdmin) UsersBy(_ context.Context, _ int64, filter, _ string, _ int) ([]AdminUser, bool, error) {
	f.filters = append(f.filters, filter)
	if f.usersByErr != nil {
		return nil, false, f.usersByErr
	}
	if f.none {
		return nil, false, nil
	}
	return []AdminUser{f.user}, false, nil
}
func (f *fakeAdmin) Users(context.Context, int64, string, int) ([]AdminUser, bool, error) {
	return []AdminUser{f.user}, true, nil
}
func (f *fakeAdmin) User(_ context.Context, _ int64, id int64) (AdminUser, error) {
	if id != f.user.ID {
		return AdminUser{}, model.ErrNotFound
	}
	return f.user, nil
}
func (f *fakeAdmin) GrantDays(_ context.Context, _, id, d int64) (AdminUser, error) {
	f.rec("days")
	_ = d
	return f.user, nil
}
func (f *fakeAdmin) SetDisabled(context.Context, int64, int64, bool) (AdminUser, error) {
	f.rec("disabled")
	return f.user, nil
}
func (f *fakeAdmin) SetTrafficLimit(_ context.Context, _, _, gb int64) (AdminUser, error) {
	f.rec("gb")
	_ = gb
	return f.user, nil
}
func (f *fakeAdmin) SetDeviceLimit(context.Context, int64, int64, int64) (AdminUser, error) {
	f.rec("devices")
	return f.user, nil
}
func (f *fakeAdmin) ResetTraffic(context.Context, int64, int64) (AdminUser, error) {
	f.rec("reset")
	return f.user, nil
}
func (f *fakeAdmin) Reissue(context.Context, int64, int64) (AdminUser, error) {
	f.rec("reissue")
	return f.user, nil
}
func (f *fakeAdmin) SetNote(_ context.Context, _, _ int64, n string) (AdminUser, error) {
	f.rec("note:" + n)
	return f.user, nil
}
func (f *fakeAdmin) Delete(context.Context, int64, int64) error { f.rec("delete"); return nil }
func (f *fakeAdmin) Tariffs(context.Context, int64) ([]AdminTariff, error) {
	return []AdminTariff{{ID: 3, Name: "Месяц", Days: 30}}, nil
}
func (f *fakeAdmin) SetTariff(context.Context, int64, int64, int64) (AdminUser, error) {
	f.rec("tariff")
	return f.user, nil
}
func (f *fakeAdmin) Nodes(context.Context, int64) ([]AdminNode, error) {
	return []AdminNode{{ID: 1, Name: "NL-1", Enabled: true, Online: true}, {ID: 2, Name: "DE-1"}}, nil
}
func (f *fakeAdmin) Node(_ context.Context, _, id int64) (AdminNodeInfo, error) {
	if f.node.ID == 0 {
		f.node = AdminNodeInfo{AdminNode: AdminNode{ID: 1, Name: "NL-<1>", Host: "nl.example", Enabled: true, Online: true, Version: "0.5.2.1-rc.10", Behind: true},
			Status: "ok", CPU: 12, MemUsed: 1 << 30, MemTotal: 4 << 30, Conns: 7, Listeners: 3, ListenersOK: 3, Inbounds: 4, Traffic24h: 5 << 30, CanUpdate: true}
	}
	if id != f.node.ID {
		return AdminNodeInfo{}, model.ErrNotFound
	}
	return f.node, nil
}
func (f *fakeAdmin) nodeAct(name string) (AdminNodeInfo, error) {
	f.rec(name)
	return f.node, f.nodeErr
}
func (f *fakeAdmin) SetNodeEnabled(_ context.Context, _, _ int64, on bool) (AdminNodeInfo, error) {
	if on {
		return f.nodeAct("node_on")
	}
	return f.nodeAct("node_off")
}
func (f *fakeAdmin) SetNodeField(_ context.Context, _, _ int64, field, v string) (AdminNodeInfo, error) {
	return f.nodeAct("node_field:" + field + "=" + v)
}
func (f *fakeAdmin) CheckNode(context.Context, int64, int64) (AdminNodeInfo, error) {
	return f.nodeAct("node_check")
}
func (f *fakeAdmin) SpeedNode(context.Context, int64, int64) (AdminSpeed, error) {
	f.rec("node_speed")
	return AdminSpeed{PingMs: 12, LossPct: 0.5, DownBps: 90e6, UpBps: 40e6}, f.nodeErr
}
func (f *fakeAdmin) UpdateNode(context.Context, int64, int64) (AdminNodeInfo, error) {
	return f.nodeAct("node_update")
}
func (f *fakeAdmin) RestartNode(context.Context, int64, int64) error {
	_, err := f.nodeAct("node_restart")
	return err
}
func (f *fakeAdmin) DeleteNode(context.Context, int64, int64) error {
	_, err := f.nodeAct("node_delete")
	return err
}
func (f *fakeAdmin) Payments(context.Context, int64, string, int) ([]AdminPayment, bool, error) {
	return []AdminPayment{{ID: 7, User: "Анна", Status: "applied", Amount: 15000, Currency: "RUB", At: time.Unix(1790000000, 0)}}, true, nil
}
func (f *fakeAdmin) Payment(context.Context, int64, int64) (AdminPaymentInfo, error) {
	return AdminPaymentInfo{AdminPayment: AdminPayment{ID: 7, User: "Анна", Status: "failed", Amount: 15000, Currency: "RUB", Provider: "stars"},
		Kind: "renew", Tariff: "Месяц", TermDays: 30, Error: "timeout", UserID: 5}, nil
}
func (f *fakeAdmin) NoteGrowth(context.Context, int64, bool, int64, int64) error { return nil }
func (f *fakeAdmin) BroadcastAudience(context.Context, int64) (int, error)       { return 3, nil }
func (f *fakeAdmin) Broadcast(context.Context, int64, string) (int, error) {
	f.rec("broadcast")
	return 3, nil
}

// shell is the Backend the screens need: its config, its brand and its admin side.
type shell struct {
	Backend
	adm *fakeAdmin
}

func (s shell) Config(context.Context) Config {
	c := Default("ru")
	c.AdminIDs = []int64{boss}
	return c
}
func (s shell) Brand(context.Context) string { return "Mikan" }
func (s shell) Admin() Admin                 { return s.adm }

func screenBot(adm *fakeAdmin) *Bot {
	return New(Deps{Backend: shell{adm: adm}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return time.Unix(1789000000, 0) }})
}

func testUser() AdminUser {
	return AdminUser{ID: 5, Name: "Анна <b>", Contact: "anna@x", State: "expiring", ExpiresAt: 1789000000 + 3*86400, Used: 3 << 30, Limit: 10 << 30,
		Devices: 2, DeviceLimit: 3, TgID: 42, Tariff: "Месяц", Note: "VIP <i>", CreatedAt: 1700000000}
}

// Every screen draws, its text is escaped and every button fits Telegram's limits and leads to
// a code the bot answers.
func TestAdminScreensDraw(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	for _, d := range []string{"a", "ao", "au:0", "au:1", "au::0", "au:s:0", "au:e:0", "au:t:0", "au:n:0", "au:b:2", "as", "ak:5", "ad:5", "ae:5:30", "ac:5:1", "ac:5:0", "ag:5", "ag:5:50", "av:5", "av:5:0",
		"ar:5", "ai:5", "ax:5", "ax:5:2", "aN:5", "at:5", "aT:5:3", "an", "ap::0", "ap:p:1", "ap:f:0", "aP:7:f:0", "ab"} {
		text, kb := b.adminScreen(ctx, boss, d, "")
		if text == "" {
			t.Errorf("%s: no text", d)
		}
		for _, bad := range []string{"<b>>", "Анна <b>", "VIP <i>"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s: unescaped %q in\n%s", d, bad, text)
			}
		}
		if ok, why := htmlBalanced(text); !ok {
			t.Errorf("%s: Telegram would refuse this text (%s):\n%s", d, why, text)
		}
		if kb == nil {
			continue
		}
		for _, row := range kb.InlineKeyboard {
			if len(row) == 0 || len(row) > 4 {
				t.Errorf("%s: a row of %d buttons", d, len(row))
			}
			for _, bt := range row {
				if bt.Text == "" || len(bt.CallbackData) == 0 || len(bt.CallbackData) > 64 {
					t.Errorf("%s: bad button %+v", d, bt)
				}
				cmd, _, _ := strings.Cut(bt.CallbackData, ":")
				if _, ok := b.pressKnown(cmd); !ok {
					t.Errorf("%s: button %q leads to a code the bot does not answer", d, bt.CallbackData)
				}
			}
		}
	}
}

// pressKnown says whether the bot answers a callback code as the admin's.
func (b *Bot) pressKnown(cmd string) (string, bool) {
	switch cmd {
	case "a", "ao", "au", "an", "ap", "aP", "ac", "ae", "ak", "ad", "as", "az", "ab", "ag", "av", "ar", "ai", "ax", "at", "aT", "aN", "aS", "aG":
		return cmd, true
	}
	return "", false
}

// The card shows what the client has and nothing that reaches the subscription.
func TestAdminCardContent(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	text, _ := screenBot(adm).adminScreen(context.Background(), boss, "ak:5", "")
	for _, want := range []string{"Скоро закончится", "ещё 3 дн.", "Месяц", "3.0 GB / 10.0 GB", "▰▰▰▱▱▱▱▱▱▱ 30%", "2 / 3", "42", "anna@x", "VIP"} {
		if !strings.Contains(text, want) {
			t.Errorf("the card lacks %q:\n%s", want, text)
		}
	}
}

// A change reaches the panel only through its confirmation, and a deletion through two.
func TestAdminChangesNeedConfirmation(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	for _, d := range []string{"ae:5:30", "ac:5:1", "ag:5:50", "av:5:2", "ar:5", "ai:5", "ax:5", "ax:5:2", "aT:5:3"} {
		b.adminScreen(ctx, boss, d, "")
	}
	if len(adm.calls) != 0 {
		t.Fatalf("asking changed something: %v", adm.calls)
	}
	for _, d := range []string{"az:e:5:30", "az:b:5:1", "az:g:5:50", "az:v:5:2", "az:r:5", "az:i:5", "az:t:5:3"} {
		b.adminScreen(ctx, boss, d, "")
	}
	if got := strings.Join(adm.calls, ","); got != "days,disabled,gb,devices,reset,reissue,tariff" {
		t.Fatalf("confirmed changes: %s", got)
	}
	text, _ := b.adminScreen(ctx, boss, "az:x:5", "")
	if adm.calls[len(adm.calls)-1] != "delete" || !strings.Contains(text, "удалена") {
		t.Fatalf("delete: %v / %s", adm.calls, text)
	}
	// An unknown kind of change does nothing.
	n := len(adm.calls)
	b.adminScreen(ctx, boss, "az:q:5:1", "")
	if len(adm.calls) != n {
		t.Fatal("an unknown change reached the panel")
	}
}

// The note goes through the held text and the confirmation; "-" clears it.
func TestAdminNoteFlow(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	b.adminScreen(ctx, boss, "aN:5", "")
	if st, ok := b.takeWait(boss); !ok || st.kind != "note" || st.user != 5 {
		t.Fatalf("the note is not awaited: %+v", st)
	}
	b.setDraft(boss, "новая")
	b.adminScreen(ctx, boss, "aN:5:s", "")
	if got := strings.Join(adm.calls, ","); got != "note:новая" {
		t.Fatalf("note: %s", got)
	}
	if _, ok := b.draftOf(boss); ok {
		t.Error("the held note stayed after it was saved")
	}
}

// Nobody but an admin gets a screen, whatever the code.
func TestAdminScreensRefuseOthers(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	for _, d := range []string{"ao", "ak:5", "az:x:5", "az:e:5:30"} {
		text, kb := b.adminScreen(context.Background(), 555, d, "")
		if kb != nil || strings.Contains(text, "Анна") {
			t.Errorf("%s for a stranger: %s", d, text)
		}
	}
	if len(adm.calls) != 0 {
		t.Fatalf("a stranger changed something: %v", adm.calls)
	}
}

var _ = tgapi.Button{}

// Deleting takes two asks: the first screen's "yes" leads to the second, never to the delete
// itself, and only the second's "yes" does it.
func TestAdminDeleteNeedsTwoAsks(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	yes := func(d string) string {
		_, kb := b.adminScreen(ctx, boss, d, "")
		if kb == nil || len(kb.InlineKeyboard) == 0 {
			t.Fatalf("%s: no keyboard", d)
		}
		return kb.InlineKeyboard[0][0].CallbackData
	}
	first := yes("ax:5")
	if strings.HasPrefix(first, "az:") {
		t.Fatalf("the first yes deletes at once: %s", first)
	}
	second := yes(first)
	if second != "az:x:5" {
		t.Fatalf("the second yes is %q, want az:x:5", second)
	}
	if len(adm.calls) != 0 {
		t.Fatalf("asking deleted: %v", adm.calls)
	}
}

// Every "yes" button of an ask leads to the change it asked about, and the "no" returns to the
// card: a mislabelled button would change the wrong thing.
func TestAdminAskButtons(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	for ask, want := range map[string]string{
		"ae:5:30": "az:e:5:30", "ac:5:1": "az:b:5:1", "ac:5:0": "az:b:5:0", "ag:5:50": "az:g:5:50", "av:5:2": "az:v:5:2",
		"ar:5": "az:r:5", "ai:5": "az:i:5", "aT:5:3": "az:t:5:3",
	} {
		_, kb := b.adminScreen(context.Background(), boss, ask, "")
		row := kb.InlineKeyboard[0]
		if row[0].CallbackData != want || row[1].CallbackData != "ak:5" {
			t.Errorf("%s: yes=%q no=%q, want %q / ak:5", ask, row[0].CallbackData, row[1].CallbackData, want)
		}
	}
}

// The clients' filters ask the panel for exactly the filter on the button, and the old
// buttons (au:<page>) of messages already sent still work.
func TestAdminUserFilters(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	for data, want := range map[string]string{"au:0": "", "au::1": "", "au:s:0": "s", "au:e:0": "e", "au:t:3": "t", "au:n:0": "n", "au:b:0": "b"} {
		adm.filters = nil
		text, kb := b.adminScreen(ctx, boss, data, "")
		if len(adm.filters) != 1 || adm.filters[0] != want {
			t.Errorf("%s asked for %q, want %q", data, adm.filters, want)
		}
		if !strings.Contains(fmt.Sprint(kb), "Анна") || kb == nil {
			t.Errorf("%s: the list is not drawn:\n%s\n%v", data, text, kb)
		}
	}
	// The next page keeps the filter.
	adm.user = testUser()
	_, kb := b.adminScreen(ctx, boss, "au:s:0", "")
	if !strings.Contains(fmt.Sprint(kb), "au:s:") {
		t.Errorf("the buttons lose the filter: %v", kb)
	}
	// Nobody matches: a word about the filter, not about the search.
	adm.none = true
	text, _ := b.adminScreen(ctx, boss, "au:s:0", "")
	if !strings.Contains(text, "фильтр") {
		t.Errorf("an empty filter says:\n%s", text)
	}
}

// A panel older than the bot has no UsersBy: the plain list still opens, and a filter says so
// instead of the bare "did not work".
func TestAdminUserFiltersOnOldPanel(t *testing.T) {
	adm := &fakeAdmin{user: testUser(), usersByErr: &RPCError{Code: "no_method", Message: "no method Admin.UsersBy"}}
	b := screenBot(adm)
	ctx := context.Background()
	if text, kb := b.adminScreen(ctx, boss, "au:0", ""); !strings.Contains(fmt.Sprint(kb), "Анна") {
		t.Errorf("the plain list does not open on an old panel:\n%s\n%v", text, kb)
	}
	if text, _ := b.adminScreen(ctx, boss, "au:s:0", ""); !strings.Contains(text, "Обновите панель") {
		t.Errorf("a filter on an old panel says:\n%s", text)
	}
}

// The overview shows the money by currency and leaves it out when the panel sends none.
func TestAdminOverviewMoney(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	text, _ := b.adminScreen(ctx, boss, "ao", "")
	if strings.Contains(text, "Выручка") || strings.Contains(text, "Новых за сутки") {
		t.Errorf("figures the panel did not send are drawn:\n%s", text)
	}
	adm.over = AdminOverview{NewToday: 3, Revenue: []AdminMoney{
		{Currency: "RUB", Today: 15000, Week: 90000, Month: 250000}, {Currency: "XTR", Today: 0, Week: 50, Month: 120}}}
	text, _ = b.adminScreen(ctx, boss, "ao", "")
	for _, want := range []string{"Выручка", "150.00 ₽", "900.00 ₽", "2500.00 ₽", "120 ⭐", "Новых за сутки: <b>3</b>"} {
		if !strings.Contains(text, want) {
			t.Errorf("the overview lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "₽ ⭐") {
		t.Errorf("currencies are added up:\n%s", text)
	}
}

// Each cause has its own sentence, and none shows the raw error.
func TestAdminFailTexts(t *testing.T) {
	aw := admWordsFor("ru")
	for _, c := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, aw.unavailable},
		{&RPCError{Code: "no_method", Message: "no method X"}, aw.failOld},
		{model.ErrAdminInput, aw.failInput},
		{model.ErrBroadcastBusy, aw.bcBusy},
		{model.ErrBroadcastOff, aw.bcOff},
		{errors.New("pq: connection refused at 10.0.0.5"), aw.failed},
	} {
		got := failText(aw, c.err)
		if got != c.want {
			t.Errorf("%v: %q, want %q", c.err, got, c.want)
		}
		if strings.Contains(got, "10.0.0.5") || strings.Contains(got, "pq:") {
			t.Errorf("the raw error leaks: %q", got)
		}
	}
}
