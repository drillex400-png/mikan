package tgbot

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"mikan/internal/panel/events"
	"mikan/internal/panel/growth"
	"mikan/internal/panel/model"
	"mikan/internal/tgapi"
)

type memKV struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *memKV) Get(_ context.Context, k string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	return v, ok, nil
}
func (s *memKV) PutIfAbsent(_ context.Context, k string, v []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[k]; ok {
		return false, nil
	}
	s.m[k] = v
	return true, nil
}
func (s *memKV) Put(_ context.Context, k string, v []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

type memBank struct {
	mu    sync.Mutex
	seen  map[string]bool
	given map[int64]int64
}

func (b *memBank) Grant(_ context.Context, sub, days int64, _, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen[key] {
		return nil
	}
	b.seen[key] = true
	b.given[sub] += days
	return nil
}

// growthShell is the admin shell with the programme on memory, and accounts to look up.
type growthShell struct {
	shell
	svc      *growth.Service
	accounts map[int64]Account
}

func (g growthShell) Growth() *growth.Service { return g.svc }
func (g growthShell) Context(_ context.Context, tg int64) (Account, error) {
	return g.accounts[tg], nil
}
func (g growthShell) User(_ context.Context, id int64) (model.User, error) {
	return model.User{ID: id, Name: "u"}, nil
}

func growthBot(t *testing.T, accounts map[int64]Account) (*Bot, *fakeAdmin, *memBank, *growth.Service) {
	t.Helper()
	adm := &fakeAdmin{user: testUser()}
	bank := &memBank{seen: map[string]bool{}, given: map[int64]int64{}}
	svc := &growth.Service{Store: &memKV{m: map[string][]byte{}}, Bank: bank}
	b := New(Deps{Backend: growthShell{shell: shell{adm: adm}, svc: svc, accounts: accounts},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return time.Unix(1789000000, 0) }, Go: func(f func()) { f() }})
	b.sayHook = func(chat int64, text string) { said[chat] = append(said[chat], text) }
	return b, adm, bank, svc
}

// said records what the bot told each chat, by the last growthBot built.
var said = map[int64][]string{}

func TestApplyGrowthChange(t *testing.T) {
	st := growth.Settings{Enabled: false, InviterDays: 7, FriendDays: 7}
	if n, ok := applyGrowthChange(st, "t"); !ok || !n.Enabled {
		t.Fatal("toggle")
	}
	if n, ok := applyGrowthChange(st, "i:14"); !ok || n.InviterDays != 14 || n.FriendDays != 7 {
		t.Fatalf("inviter %+v", n)
	}
	if n, ok := applyGrowthChange(st, "f:0"); !ok || n.FriendDays != 0 {
		t.Fatalf("friend %+v", n)
	}
	// Forged or pointless callbacks change nothing.
	for _, bad := range []string{"", "i:999", "i:-1", "i:x", "f:8", "z", "i", "i:7", "t:1", "i:7:7"} {
		if n, ok := applyGrowthChange(st, bad); ok || n != st {
			t.Errorf("%q changed %+v", bad, n)
		}
	}
}

func TestGrowthScreenSavesAfterAudit(t *testing.T) {
	b, adm, _, svc := growthBot(t, nil)
	ctx := context.Background()
	text, kb := b.adminScreen(ctx, boss, "aG:t", "")
	st, _ := svc.Settings(ctx)
	if !st.Enabled {
		t.Fatalf("not switched on: %+v", st)
	}
	if !strings.Contains(text, "включена") || kb == nil {
		t.Fatalf("screen:\n%s", text)
	}
	b.adminScreen(ctx, boss, "aG:i:14", "")
	if st, _ = svc.Settings(ctx); st.InviterDays != 14 {
		t.Fatalf("reward not saved: %+v", st)
	}
	_ = adm
}

func TestGrowthScreenButtonsAreKnown(t *testing.T) {
	b, _, _, _ := growthBot(t, nil)
	_, kb := b.adminScreen(context.Background(), boss, "aG", "")
	for _, row := range kb.InlineKeyboard {
		for _, bt := range row {
			cmd, _, _ := strings.Cut(bt.CallbackData, ":")
			if _, ok := b.pressKnown(cmd); !ok || len(bt.CallbackData) > 64 || bt.Text == "" {
				t.Errorf("bad button %+v", bt)
			}
		}
	}
}

func TestGrowthScreenIsAdminsOnly(t *testing.T) {
	b, _, _, svc := growthBot(t, nil)
	ctx := context.Background()
	b.adminScreen(ctx, 999, "aG:t", "")
	if st, _ := svc.Settings(ctx); st.Enabled {
		t.Fatal("a stranger switched the programme on")
	}
}

func TestRewardOnFirstPaymentOnly(t *testing.T) {
	ctx := context.Background()
	said = map[int64][]string{}
	acc := map[int64]Account{100: {Links: []model.User{{ID: 1000}}}}
	b, _, bank, svc := growthBot(t, acc)
	svc.SaveSettings(ctx, growth.Settings{Enabled: true, InviterDays: 7, FriendDays: 3})
	if r, _ := svc.Accept(ctx, 200, growth.Link(100), false); r != growth.Counted {
		t.Fatal("not counted")
	}
	first := events.Event{Type: events.PaymentPaid, TgID: 200, UserID: 2000}
	// A renewal, a package or a repeat of the same event never pays twice.
	for _, p := range []events.Payment{{Kind: "renew"}, {Kind: "package"}, {Kind: "new", Created: false}} {
		if err := b.rewardFirstPayment(ctx, first, p); err != nil {
			t.Fatal(err)
		}
	}
	if len(bank.given) != 0 {
		t.Fatalf("paid for something that is not a first payment: %v", bank.given)
	}
	for i := 0; i < 3; i++ {
		if err := b.rewardFirstPayment(ctx, first, events.Payment{Kind: "new", Created: true}); err != nil {
			t.Fatal(err)
		}
	}
	if bank.given[1000] != 7 || bank.given[2000] != 3 {
		t.Fatalf("given %v", bank.given)
	}
	// Each side is told once, however often the event came.
	if len(said[100]) != 1 || len(said[200]) != 1 || !strings.Contains(said[100][0], "7") || !strings.Contains(said[200][0], "3") {
		t.Fatalf("said %v", said)
	}
}

func TestStartInvitationOnlyForNewcomers(t *testing.T) {
	ctx := context.Background()
	acc := map[int64]Account{300: {Links: []model.User{{ID: 3000}}}}
	b, _, _, svc := growthBot(t, acc)
	svc.SaveSettings(ctx, growth.Settings{Enabled: true, InviterDays: 7, FriendDays: 7})
	if !b.startInvitation(ctx, 200, growth.Link(100)) {
		t.Fatal("an invitation was not recognised")
	}
	if who, ok, _ := svc.InviterOf(ctx, 200); !ok || who != 100 {
		t.Fatalf("newcomer not tied: %d %v", who, ok)
	}
	b.startInvitation(ctx, 300, growth.Link(100)) // already a customer
	if _, ok, _ := svc.InviterOf(ctx, 300); ok {
		t.Fatal("a customer was counted as invited")
	}
	if b.startInvitation(ctx, 400, "c_77") || b.startInvitation(ctx, 400, "abc123") {
		t.Fatal("another code taken for an invitation")
	}
}

func TestInviteCommandOffWhenProgrammeOff(t *testing.T) {
	b, _, _, svc := growthBot(t, nil)
	ctx := context.Background()
	if b.inviteOn(ctx) {
		t.Fatal("on by default")
	}
	cs, key := b.commandMenu(ctx, false)
	if len(cs) != 1 || strings.Contains(key, "invite") {
		t.Fatalf("menu %v %q", cs, key)
	}
	svc.SaveSettings(ctx, growth.Settings{Enabled: true, InviterDays: 7, FriendDays: 7})
	cs, key = b.commandMenu(ctx, true)
	names := []string{}
	for _, c := range cs {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "start,invite,admin" || !strings.Contains(key, "invite") {
		t.Fatalf("menu %v %q", names, key)
	}
}

var _ = tgapi.Command{}
