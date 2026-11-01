package botpanel

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/model"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgbot"
)

// pay records one applied payment of amount in cur, applied at when.
func pay(t *testing.T, e *env, payload, cur string, amount int64, when time.Time) {
	t.Helper()
	tariffs, err := e.st.Q.ListTariffs(e.ctx)
	if err != nil || len(tariffs) == 0 {
		t.Fatal("no tariffs", err)
	}
	p, err := e.st.Q.CreatePayment(e.ctx, db.CreatePaymentParams{Provider: "stars", Payload: payload, Kind: "new", TariffID: sql.NullInt64{Int64: tariffs[1].ID, Valid: true},
		TariffName: tariffs[1].Name, Amount: amount, Currency: cur, CreatedAt: when.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Q.MarkPaymentPaid(e.ctx, db.MarkPaymentPaidParams{ID: p.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Q.MarkPaymentApplied(e.ctx, db.MarkPaymentAppliedParams{UserID: sql.NullInt64{Int64: e.user.ID, Valid: true}, AppliedAt: sql.NullInt64{Int64: when.Unix(), Valid: true}, ID: p.ID}); err != nil {
		t.Fatal(err)
	}
}

// The filters ask for the right clients, refuse an unknown one, and are the admin's alone.
func TestAdminUsersByFilters(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()

	if _, _, err := a.UsersBy(ctx, adminTg+1, "", "", 0); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("a stranger: %v", err)
	}
	if _, _, err := a.UsersBy(ctx, adminTg, "zzz", "", 0); !errors.Is(err, model.ErrAdminInput) {
		t.Fatalf("an unknown filter: %v", err)
	}
	all, _, err := a.UsersBy(ctx, adminTg, tgbot.FilterAll, "", 0)
	if err != nil || len(all) == 0 {
		t.Fatalf("all: %d %v", len(all), err)
	}
	plain, _, _ := a.Users(ctx, adminTg, "", 0)
	if len(plain) != len(all) {
		t.Fatalf("the unfiltered list differs from the plain one: %d and %d", len(all), len(plain))
	}

	// A client with nothing special is in none of the narrow filters.
	for _, f := range []string{tgbot.FilterEnded, tgbot.FilterBlocked, tgbot.FilterLowData} {
		got, _, err := a.UsersBy(ctx, adminTg, f, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range got {
			if u.ID == e.user.ID {
				t.Errorf("filter %q holds a client who is none of it", f)
			}
		}
	}

	// Blocked.
	if _, err := a.SetDisabled(ctx, adminTg, e.user.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _, _ := a.UsersBy(ctx, adminTg, tgbot.FilterBlocked, "", 0)
	if len(got) != 1 || got[0].ID != e.user.ID {
		t.Fatalf("blocked: %+v", got)
	}
	if _, err := a.SetDisabled(ctx, adminTg, e.user.ID, false); err != nil {
		t.Fatal(err)
	}

	// Nearly out of traffic: 95% of a 10 GB limit.
	if _, err := a.SetTrafficLimit(ctx, adminTg, e.user.ID, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Q.ListUsers(ctx); err != nil {
		t.Fatal(err)
	}
	u, _ := e.st.Q.GetUser(ctx, e.user.ID)
	u.UsedDown = 95 * (10 << 30) / 100
	if !matches(tgbot.FilterLowData, u, domain.State(u, 0, e.now)) {
		t.Error("95% of the limit is not low data")
	}
	u.UsedDown = 50 * (10 << 30) / 100
	if matches(tgbot.FilterLowData, u, domain.State(u, 0, e.now)) {
		t.Error("50% of the limit is low data")
	}
	u.TrafficLimit = sql.NullInt64{}
	u.UsedDown = 1 << 40
	if matches(tgbot.FilterLowData, u, domain.State(u, 0, e.now)) {
		t.Error("a client with no limit is low data")
	}

	// Pages: the page after the last is empty, not an error.
	if past, more, err := a.UsersBy(ctx, adminTg, "", "", 50); err != nil || len(past) != 0 || more {
		t.Fatalf("a page past the end: %d %v %v", len(past), more, err)
	}
	// The search narrows a filter too.
	if none, _, _ := a.UsersBy(ctx, adminTg, "", "no-such-name-zzz", 0); len(none) != 0 {
		t.Fatalf("a search that matches nobody found %d", len(none))
	}
}

// Whom the term filters show first: the soonest to end.
func TestAdminUsersByTermOrder(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	tariffs, _ := e.st.Q.ListTariffs(ctx)
	mk := func(name string, days int64) int64 {
		u, err := e.users.Create(ctx, domain.CreateInput{Name: name, TariffID: tariffs[1].ID})
		if err != nil {
			t.Fatal(err)
		}
		end := e.now.Add(time.Duration(days) * 24 * time.Hour).Unix()
		if _, err := e.st.DB.ExecContext(ctx, "UPDATE users SET expires_at = $1 WHERE id = $2", end, u.ID); err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	late, early, ended := mk("late", 6), mk("early", 1), mk("ended", -3)
	soon, _, err := a.UsersBy(ctx, adminTg, tgbot.FilterSoon, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(soon) < 2 || soon[0].ID != early || soon[1].ID != late {
		t.Fatalf("soon, soonest first: %+v", soon)
	}
	over, _, _ := a.UsersBy(ctx, adminTg, tgbot.FilterEnded, "", 0)
	found := false
	for _, u := range over {
		if u.ID == ended {
			found = true
		}
		if u.ID == early || u.ID == late {
			t.Errorf("a running term is in the ended ones: %d", u.ID)
		}
	}
	if !found {
		t.Error("the ended client is not in the ended filter")
	}
}

// The money is by currency and by period, nothing is added across currencies, and a payment
// outside a period stays out of it.
func TestAdminOverviewRevenue(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	now := e.clock()
	pay(t, e, "a", "RUB", 10000, now)                        // today
	pay(t, e, "b", "RUB", 25000, now.Add(-3*24*time.Hour))   // this week
	pay(t, e, "c", "XTR", 50, now)                           // today, other currency
	pay(t, e, "d", "RUB", 99900, now.Add(-400*24*time.Hour)) // long ago: in no period
	o, err := a.Overview(ctx, adminTg)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]tgbot.AdminMoney{}
	for _, m := range o.Revenue {
		by[m.Currency] = m
	}
	if len(by) != 2 {
		t.Fatalf("currencies: %+v", o.Revenue)
	}
	if r := by["RUB"]; r.Today != 10000 || r.Week != 35000 || r.Month < 10000 || r.Month > 35000 {
		t.Errorf("RUB: %+v", r)
	}
	if x := by["XTR"]; x.Today != 50 || x.Week != 50 {
		t.Errorf("XTR: %+v", x)
	}
	if o.ExpireSoon != o.Expiring {
		t.Errorf("the expiring figure differs: %d and %d", o.ExpireSoon, o.Expiring)
	}
	if o.NewToday < 1 {
		t.Errorf("the client made a moment ago is not new today: %d", o.NewToday)
	}
	if _, err := e.local.Admin().Overview(ctx, adminTg+1); !errors.Is(err, model.ErrNotAdmin) {
		t.Errorf("a stranger sees the money: %v", err)
	}
}
