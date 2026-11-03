package botpanel

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"mikan/internal/panel/model"
	"mikan/internal/panel/tgbot"
)

func TestAdminPromoCodes(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	adm := e.local.Admin()
	if _, err := adm.CreatePromo(e.ctx, adminTg+1, tgbot.AdminPromoInput{Type: "days", Value: 7}); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("a stranger: %v", err)
	}
	for _, bad := range []tgbot.AdminPromoInput{
		{Type: "days", Value: 0}, {Type: "days", Value: 7, Currency: "RUB"}, {Type: "percent", Value: 101},
		{Type: "traffic", Value: 1}, {Type: "fixed", Value: 500}, {Type: "days", Value: 7, Code: "с1"},
		{Type: "days", Value: 7, MaxUses: -1}, {Type: "nope", Value: 1},
	} {
		if _, err := adm.CreatePromo(e.ctx, adminTg, bad); !errors.Is(err, model.ErrAdminInput) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	p, err := adm.CreatePromo(e.ctx, adminTg, tgbot.AdminPromoInput{Code: " spring26 ", Type: "percent", Value: 20, MaxUses: 50, Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	if p.Code != "SPRING26" || !p.Enabled || p.MaxUses != 50 || p.EndsAt != e.now.Unix()+30*86400 || p.PerUser != 1 {
		t.Fatalf("created %+v", p)
	}
	if _, err := adm.CreatePromo(e.ctx, adminTg, tgbot.AdminPromoInput{Code: "SPRING26", Type: "days", Value: 7}); !errors.Is(err, model.ErrAdminInput) {
		t.Fatalf("a taken code: %v", err)
	}
	r, err := adm.CreatePromo(e.ctx, adminTg, tgbot.AdminPromoInput{Type: "fixed", Value: 10000, Currency: "RUB"})
	if err != nil || len(r.Code) != 8 {
		t.Fatalf("a random code: %+v %v", r, err)
	}
	if p, err = adm.SetPromoEnabled(e.ctx, adminTg, p.ID, false); err != nil || p.Enabled {
		t.Fatalf("off: %+v %v", p, err)
	}
	list, _, err := adm.Promos(e.ctx, adminTg, 0)
	if err != nil || len(list) != 2 || list[0].ID != r.ID {
		t.Fatalf("list %+v %v", list, err)
	}
	st, err := adm.PromoStats(e.ctx, adminTg)
	if err != nil || st.Codes != 2 || st.Active != 1 {
		t.Fatalf("stats %+v %v", st, err)
	}
	if uses, more, err := adm.PromoUses(e.ctx, adminTg, p.ID, 0); err != nil || len(uses) != 0 || more {
		t.Fatalf("uses %+v %v %v", uses, more, err)
	}
	if err := adm.DeletePromo(e.ctx, adminTg, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := adm.Promo(e.ctx, adminTg, p.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("a removed code: %v", err)
	}
	acts := auditActions(t, e)
	for _, want := range []string{"tgadmin.promo_create", "tgadmin.promo_enabled", "tgadmin.promo_delete"} {
		if !slices.Contains(acts, want) {
			t.Errorf("the log lacks %s: %v", want, acts)
		}
	}
	if !strings.Contains(auditDump(t, e), "tg:424242") {
		t.Error("the log lacks the admin")
	}
}

func TestAdminTariffs(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	adm := e.local.Admin()
	ts, err := adm.Tariffs(e.ctx, adminTg)
	if err != nil || len(ts) < 2 {
		t.Fatalf("tariffs %v %v", ts, err)
	}
	id := ts[1].ID
	before, err := adm.Tariff(e.ctx, adminTg, id)
	if err != nil {
		t.Fatal(err)
	}
	if before.Users != 1 {
		t.Fatalf("the seeded client is on it: %+v", before)
	}
	if _, err := adm.SetTariffPrice(e.ctx, adminTg+1, id, "RUB", 100); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("a stranger: %v", err)
	}
	for _, bad := range []struct {
		cur string
		n   int64
	}{{"RUB", -1}, {"USD", 100}, {"XTR", tgbot.MaxPriceStars + 1}} {
		if _, err := adm.SetTariffPrice(e.ctx, adminTg, id, bad.cur, bad.n); !errors.Is(err, model.ErrAdminInput) {
			t.Errorf("%v: %v", bad, err)
		}
	}
	got, err := adm.SetTariffPrice(e.ctx, adminTg, id, "RUB", 34990)
	if err != nil || got.PriceRub != 34990 || got.Name != before.Name || got.Days != before.Days || got.Limit != before.Limit {
		t.Fatalf("price %+v %v (before %+v)", got, err, before)
	}
	if got, err = adm.SetTariffPrice(e.ctx, adminTg, id, "XTR", 0); err != nil || got.PriceStars != 0 || got.PriceRub != 34990 {
		t.Fatalf("stars off %+v %v", got, err)
	}
	if got, err = adm.SetTariffOnSale(e.ctx, adminTg, id, !before.OnSale); err != nil || got.OnSale == before.OnSale {
		t.Fatalf("sale %+v %v", got, err)
	}
	if _, err := adm.Tariff(e.ctx, adminTg, 999999); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("no tariff: %v", err)
	}
	acts := auditActions(t, e)
	for _, want := range []string{"tgadmin.tariff_price", "tgadmin.tariff_sale"} {
		if !slices.Contains(acts, want) {
			t.Errorf("the log lacks %s: %v", want, acts)
		}
	}
}

func TestAdminTops(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	adm := e.local.Admin()
	if _, err := adm.Tops(e.ctx, adminTg, "nope"); !errors.Is(err, model.ErrAdminInput) {
		t.Fatalf("a bad kind: %v", err)
	}
	if _, err := e.st.DB.ExecContext(e.ctx, "UPDATE users SET used_up = $1, used_down = $1, device_limit = 2 WHERE id = $2", int64(1<<30), e.user.ID); err != nil {
		t.Fatal(err)
	}
	top, err := adm.Tops(e.ctx, adminTg, tgbot.TopTraffic)
	if err != nil || len(top) != 1 || top[0].ID != e.user.ID || top[0].Value != 2<<30 {
		t.Fatalf("traffic %+v %v", top, err)
	}
	if top, err = adm.Tops(e.ctx, adminTg, tgbot.TopShare); err != nil || len(top) != 0 {
		t.Fatalf("nobody shares yet: %+v %v", top, err)
	}
	for i, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.3"} {
		_, err := e.st.DB.ExecContext(e.ctx, `INSERT INTO bound_devices (user_id, hwid, slot_id, last_ip, created_at, last_seen)
SELECT id, $2, slot_id, $3, $4, $4 FROM users WHERE id = $1`, e.user.ID, "hw"+string(rune('a'+i)), ip, e.now.Unix())
		if err != nil {
			t.Fatal(err)
		}
	}
	top, err = adm.Tops(e.ctx, adminTg, tgbot.TopShare)
	if err != nil || len(top) != 1 || top[0].IPs != 3 || top[0].Devices != 4 || top[0].Score != 3*2+2*3 {
		t.Fatalf("shared %+v %v", top, err)
	}
	if top, err = adm.Tops(e.ctx, adminTg, tgbot.TopMoney); err != nil || len(top) != 0 {
		t.Fatalf("nobody paid: %+v %v", top, err)
	}
}
