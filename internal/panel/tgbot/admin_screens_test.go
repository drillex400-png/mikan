package tgbot

import (
	"context"
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
}

func (f *fakeAdmin) rec(s string) { f.calls = append(f.calls, s) }

func (f *fakeAdmin) Overview(context.Context, int64) (AdminOverview, error) {
	return AdminOverview{Users: 10, Active: 6, Expiring: 2, Expired: 1, Disabled: 1, Nodes: 2, NodesOnline: 1, PaidMonth: 4}, nil
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
	return []AdminNode{{ID: 1, Name: "NL-1", Enabled: true}, {ID: 2, Name: "DE-1"}}, nil
}
func (f *fakeAdmin) Payments(context.Context, int64, string, int) ([]AdminPayment, bool, error) {
	return []AdminPayment{{ID: 7, User: "Анна", Status: "applied", Amount: 15000, Currency: "RUB", At: time.Unix(1790000000, 0)}}, true, nil
}
func (f *fakeAdmin) Payment(context.Context, int64, int64) (AdminPaymentInfo, error) {
	return AdminPaymentInfo{AdminPayment: AdminPayment{ID: 7, User: "Анна", Status: "failed", Amount: 15000, Currency: "RUB", Provider: "stars"},
		Kind: "renew", Tariff: "Месяц", TermDays: 30, Error: "timeout", UserID: 5}, nil
}
func (f *fakeAdmin) BroadcastAudience(context.Context, int64) (int, error) { return 3, nil }
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
	for _, d := range []string{"a", "ao", "au:0", "au:1", "as", "ak:5", "ad:5", "ae:5:30", "ac:5:1", "ac:5:0", "ag:5", "ag:5:50", "av:5", "av:5:0",
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
	case "a", "ao", "au", "an", "ap", "aP", "ac", "ae", "ak", "ad", "as", "az", "ab", "ag", "av", "ar", "ai", "ax", "at", "aT", "aN":
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
