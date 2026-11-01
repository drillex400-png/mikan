package growth

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type memStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newStore() *memStore { return &memStore{m: map[string][]byte{}} }
func (s *memStore) Get(_ context.Context, k string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	return v, ok, nil
}
func (s *memStore) PutIfAbsent(_ context.Context, k string, v []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[k]; ok {
		return false, nil
	}
	s.m[k] = v
	return true, nil
}
func (s *memStore) Put(_ context.Context, k string, v []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

// bank behaves like the panel's grant call: one key gives once.
type bank struct {
	mu    sync.Mutex
	seen  map[string]bool
	given map[int64]int64
	fail  map[string]bool // keys that fail once
}

func newBank() *bank {
	return &bank{seen: map[string]bool{}, given: map[int64]int64{}, fail: map[string]bool{}}
}
func (b *bank) Grant(_ context.Context, sub, days int64, _ string, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail[key] {
		delete(b.fail, key)
		return errors.New("panel down")
	}
	if b.seen[key] {
		return nil
	}
	b.seen[key] = true
	b.given[sub] += days
	return nil
}

func on(t *testing.T, inv, friend int64) (*Service, *bank) {
	t.Helper()
	st, bk := newStore(), newBank()
	s := &Service{Store: st, Bank: bk}
	if err := s.SaveSettings(context.Background(), Settings{Enabled: true, InviterDays: inv, FriendDays: friend}); err != nil {
		t.Fatal(err)
	}
	return s, bk
}

func TestParseCode(t *testing.T) {
	for code, want := range map[string]int64{"r_123": 123, " r_9 ": 9} {
		if got, ok := ParseCode(code); !ok || got != want {
			t.Errorf("%q = %d %v", code, got, ok)
		}
	}
	for _, bad := range []string{"", "r_", "r_0", "r_-5", "r_abc", "c_12", "r_12x", "r_99999999999999999999", "123"} {
		if _, ok := ParseCode(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestAcceptRules(t *testing.T) {
	ctx := context.Background()
	s, _ := on(t, 7, 7)
	if r, _ := s.Accept(ctx, 2, Link(1), false); r != Counted {
		t.Fatalf("new friend = %v", r)
	}
	if r, _ := s.Accept(ctx, 2, Link(3), false); r != AlreadyIn {
		t.Fatalf("second inviter = %v", r)
	}
	if who, _, _ := s.InviterOf(ctx, 2); who != 1 {
		t.Fatalf("inviter changed to %d", who)
	}
	if r, _ := s.Accept(ctx, 5, Link(5), false); r != NotCounted {
		t.Fatalf("own link counted: %v", r)
	}
	if r, _ := s.Accept(ctx, 6, Link(1), true); r != NotCounted {
		t.Fatalf("existing customer counted: %v", r)
	}
	if r, _ := s.Accept(ctx, 7, "c_77", false); r != NotCounted {
		t.Fatalf("campaign code counted: %v", r)
	}
}

func TestOffProgrammeCountsNothing(t *testing.T) {
	ctx := context.Background()
	s := &Service{Store: newStore(), Bank: newBank()}
	if r, _ := s.Accept(ctx, 2, Link(1), false); r != NotCounted {
		t.Fatalf("default is off, got %v", r)
	}
}

func TestPayGivesOnceWhateverTheRepeats(t *testing.T) {
	ctx := context.Background()
	s, bk := on(t, 7, 3)
	s.Accept(ctx, 2, Link(1), false)
	for i := 0; i < 4; i++ {
		if _, err := s.Pay(ctx, 2, 200, 100); err != nil {
			t.Fatal(err)
		}
	}
	if bk.given[100] != 7 || bk.given[200] != 3 {
		t.Fatalf("given %v", bk.given)
	}
	if n, _ := s.Invited(ctx, 1); n != 1 {
		t.Fatalf("counted %d", n)
	}
}

func TestPayRetryAfterAFailureDoesNotDoubleGive(t *testing.T) {
	ctx := context.Background()
	s, bk := on(t, 7, 7)
	s.Accept(ctx, 2, Link(1), false)
	bk.fail["ref-inviter-2"] = true // the friend's grant goes through, the inviter's fails
	if _, err := s.Pay(ctx, 2, 200, 100); err == nil {
		t.Fatal("expected the failure")
	}
	if _, err := s.Pay(ctx, 2, 200, 100); err != nil {
		t.Fatal(err)
	}
	if bk.given[200] != 7 || bk.given[100] != 7 {
		t.Fatalf("given %v", bk.given)
	}
}

func TestPayForNobodysFriendIsNothing(t *testing.T) {
	s, bk := on(t, 7, 7)
	p, err := s.Pay(context.Background(), 9, 900, 100)
	if err != nil || p.Rewarded || len(bk.given) != 0 {
		t.Fatalf("%v %v %v", p, err, bk.given)
	}
}

func TestPayWhenSwitchedOffAfterInvitation(t *testing.T) {
	ctx := context.Background()
	s, bk := on(t, 7, 7)
	s.Accept(ctx, 2, Link(1), false)
	s.SaveSettings(ctx, Settings{Enabled: false, InviterDays: 7, FriendDays: 7})
	if p, _ := s.Pay(ctx, 2, 200, 100); p.Rewarded || len(bk.given) != 0 {
		t.Fatalf("paid while off: %v", bk.given)
	}
}

func TestInviterWithoutSubscriptionStillRewardsFriend(t *testing.T) {
	ctx := context.Background()
	s, bk := on(t, 7, 5)
	s.Accept(ctx, 2, Link(1), false)
	p, _ := s.Pay(ctx, 2, 200, 0)
	if p.InviterD != 0 || p.FriendD != 5 || bk.given[200] != 5 {
		t.Fatalf("%+v %v", p, bk.given)
	}
}

func TestSettingsBounds(t *testing.T) {
	ctx := context.Background()
	s := &Service{Store: newStore(), Bank: newBank()}
	for _, bad := range []Settings{{InviterDays: -1}, {FriendDays: MaxRewardDays + 1}} {
		if s.SaveSettings(ctx, bad) == nil {
			t.Errorf("%+v saved", bad)
		}
	}
}

func TestRepeatedPayDoesNotAnnounceAgain(t *testing.T) {
	ctx := context.Background()
	s, _ := on(t, 7, 7)
	s.Accept(ctx, 2, Link(1), false)
	first, _ := s.Pay(ctx, 2, 200, 100)
	again, _ := s.Pay(ctx, 2, 200, 100)
	if !first.Rewarded || again.Rewarded || again.InviterD != 0 || again.FriendD != 0 {
		t.Fatalf("first %+v again %+v", first, again)
	}
}
