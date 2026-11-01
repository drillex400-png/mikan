// Package growth is the referral programme of the subscribers' bot. It keeps no data of its
// own: who invited whom and what was paid out live in the addon's KV store of the panel, and
// the days are given through the Panel API's grant call, which audits them and never gives
// twice for one key. Everything that decides money is here and nowhere else, so that it can be
// tested without Telegram and without a panel.
package growth

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// CodePrefix starts the /start code of an invitation: t.me/<bot>?start=r_<inviter's id>.
const CodePrefix = "r_"

// Limits of the reward the admin sets.
const (
	MaxRewardDays = 365
	DefaultDays   = 7
)

// Settings are the admin's: whether the programme runs and what each side gets.
type Settings struct {
	Enabled     bool  `json:"enabled"`
	InviterDays int64 `json:"inviter_days"`
	FriendDays  int64 `json:"friend_days"`
}

// Valid says whether the settings can be saved.
func (s Settings) Valid() bool {
	in := func(d int64) bool { return d >= 0 && d <= MaxRewardDays }
	return in(s.InviterDays) && in(s.FriendDays)
}

// Default is what a panel that was never set up runs: off, with the usual reward ready.
func Default() Settings {
	return Settings{Enabled: false, InviterDays: DefaultDays, FriendDays: DefaultDays}
}

// Store is the addon's KV store as growth uses it.
type Store interface {
	// Get returns the value of key; found is false when there is none.
	Get(ctx context.Context, key string) (value []byte, found bool, err error)
	// PutIfAbsent stores value unless the key exists; stored says which happened.
	PutIfAbsent(ctx context.Context, key string, value []byte) (stored bool, err error)
	Put(ctx context.Context, key string, value []byte) error
}

// Granter gives days to a subscription. key makes a retry harmless: the same key gives once.
type Granter interface {
	Grant(ctx context.Context, userID, days int64, reason, key string) error
}

// Service is the programme.
type Service struct {
	Store Store
	Bank  Granter
}

// ParseCode reads the inviter's Telegram id from a /start code ("r_123"). ok is false for any
// other code, a malformed one included.
func ParseCode(code string) (inviter int64, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(code), CodePrefix)
	if !found || rest == "" || len(rest) > 18 {
		return 0, false
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// Link is the invitation link's /start code for an account.
func Link(tgID int64) string { return CodePrefix + strconv.FormatInt(tgID, 10) }

func keyInvitedBy(friend int64) string { return "ref/by/" + strconv.FormatInt(friend, 10) }
func keyPaid(friend int64) string      { return "ref/paid/" + strconv.FormatInt(friend, 10) }
func keyCount(inviter int64) string    { return "ref/count/" + strconv.FormatInt(inviter, 10) }

// Settings reads the admin's settings; absent ones are Default.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	raw, found, err := s.Store.Get(ctx, "ref/settings")
	if err != nil {
		return Settings{}, err
	}
	if !found {
		return Default(), nil
	}
	var st Settings
	if err := json.Unmarshal(raw, &st); err != nil || !st.Valid() {
		return Default(), nil
	}
	return st, nil
}

// SaveSettings stores the admin's settings.
func (s *Service) SaveSettings(ctx context.Context, st Settings) error {
	if !st.Valid() {
		return errors.New("growth: reward out of range")
	}
	raw, _ := json.Marshal(st)
	return s.Store.Put(ctx, "ref/settings", raw)
}

// Invitation is what Accept decided.
type Invitation int

const (
	NotCounted Invitation = iota // the programme is off, the code is bad, or the link is the account's own
	Counted                      // the friend now belongs to the inviter
	AlreadyIn                    // the friend was invited before: the first inviter stays
)

// Accept ties a friend to the inviter whose code the friend started the bot with. It counts
// only an account that has nothing yet (hasSubscription false): somebody who is a customer
// already cannot be "invited", and the first inviter keeps the friend.
func (s *Service) Accept(ctx context.Context, friend int64, code string, hasSubscription bool) (Invitation, error) {
	inviter, ok := ParseCode(code)
	if !ok || inviter == friend || friend <= 0 || hasSubscription {
		return NotCounted, nil
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return NotCounted, err
	}
	if !st.Enabled {
		return NotCounted, nil
	}
	stored, err := s.Store.PutIfAbsent(ctx, keyInvitedBy(friend), []byte(strconv.FormatInt(inviter, 10)))
	if err != nil {
		return NotCounted, err
	}
	if !stored {
		return AlreadyIn, nil
	}
	return Counted, nil
}

// Reward is what a first payment earned: who gets how many days, on which subscription.
type Reward struct {
	Inviter    int64 // Telegram account of the one who invited
	InviterSub int64 // the subscription the inviter's days go to; 0 when they have none to give to
	InviterDay int64
	FriendDay  int64
}

// InviterOf is who invited a friend; found is false for one nobody invited.
func (s *Service) InviterOf(ctx context.Context, friend int64) (int64, bool, error) {
	raw, found, err := s.Store.Get(ctx, keyInvitedBy(friend))
	if err != nil || !found {
		return 0, false, err
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, false, nil
	}
	return n, true, nil
}

// Paid is the result of Pay.
type Paid struct {
	Rewarded bool  // days were given now or by an earlier try of the same payment
	Inviter  int64 // the inviter's Telegram account
	Friend   int64
	InviterD int64 // days the inviter got
	FriendD  int64 // days the friend got
}

// Pay rewards a friend's FIRST payment. It is called for a payment that created the
// friend's subscription (events.Payment.Created, kind "new"); a renewal never reaches it. It
// is safe to call again for the same friend after a crash or a repeated event: each grant
// goes under a key made of the friend's account, so the panel gives once, and the "paid"
// mark is written only after both grants went through.
//
// friendSub is the subscription the payment made; inviterSub is the inviter's subscription to
// extend (0 when the inviter has none: they are told, and nothing is given).
func (s *Service) Pay(ctx context.Context, friend, friendSub, inviterSub int64) (Paid, error) {
	inviter, invited, err := s.InviterOf(ctx, friend)
	if err != nil || !invited {
		return Paid{}, err
	}
	if _, done, err := s.Store.Get(ctx, keyPaid(friend)); err != nil || done {
		// Paid before: nothing more, and Rewarded stays false so nobody is told twice.
		return Paid{Inviter: inviter, Friend: friend}, err
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return Paid{}, err
	}
	if !st.Enabled {
		return Paid{}, nil
	}
	out := Paid{Inviter: inviter, Friend: friend}
	fk := "ref-friend-" + strconv.FormatInt(friend, 10)
	ik := "ref-inviter-" + strconv.FormatInt(friend, 10)
	if st.FriendDays > 0 && friendSub > 0 {
		if err := s.Bank.Grant(ctx, friendSub, st.FriendDays, "referral: welcome bonus", fk); err != nil {
			return Paid{}, err
		}
		out.FriendD = st.FriendDays
	}
	if st.InviterDays > 0 && inviterSub > 0 {
		if err := s.Bank.Grant(ctx, inviterSub, st.InviterDays, "referral: friend "+strconv.FormatInt(friend, 10)+" paid", ik); err != nil {
			return Paid{}, err
		}
		out.InviterD = st.InviterDays
	}
	if _, err := s.Store.PutIfAbsent(ctx, keyPaid(friend), []byte("1")); err != nil {
		return Paid{}, err
	}
	n, err := s.bump(ctx, inviter)
	if err != nil {
		return Paid{}, err
	}
	_ = n
	out.Rewarded = out.FriendD > 0 || out.InviterD > 0
	return out, nil
}

// bump counts a paid friend for the inviter. A lost race may miscount by one at worst, the
// money having been given once by the keys above; the number is for show.
func (s *Service) bump(ctx context.Context, inviter int64) (int64, error) {
	raw, _, err := s.Store.Get(ctx, keyCount(inviter))
	if err != nil {
		return 0, err
	}
	n, _ := strconv.ParseInt(string(raw), 10, 64)
	n++
	return n, s.Store.Put(ctx, keyCount(inviter), []byte(strconv.FormatInt(n, 10)))
}

// Invited is how many friends of the inviter have paid.
func (s *Service) Invited(ctx context.Context, inviter int64) (int64, error) {
	raw, found, err := s.Store.Get(ctx, keyCount(inviter))
	if err != nil || !found {
		return 0, err
	}
	n, _ := strconv.ParseInt(string(raw), 10, 64)
	return n, nil
}
