package tgbot

import (
	"context"
	"strings"
	"testing"
)

func testPromo() AdminPromo {
	return AdminPromo{ID: 4, Code: "SPRING<b>", Name: "Весна <i>", Type: "days", Value: 30, Uses: 12, MaxUses: 100, PerUser: 1,
		EndsAt: 1789000000 + 10*86400, Enabled: true, FirstOnly: true, Tariffs: 2, CreatedAt: 1700000000}
}

func (f *fakeAdmin) Promos(context.Context, int64, int) ([]AdminPromo, bool, error) {
	off := testPromo()
	off.ID, off.Code, off.Type, off.Value, off.Currency, off.Enabled, off.MaxUses = 5, "MINUS", "fixed", 10000, "RUB", false, 0
	return []AdminPromo{testPromo(), off}, true, nil
}
func (f *fakeAdmin) PromoStats(context.Context, int64) (AdminPromoStats, error) {
	return AdminPromoStats{Codes: 2, Active: 1, Uses: 12, Days: 360, Bytes: 50 << 30, Orders: 3}, nil
}
func (f *fakeAdmin) Promo(context.Context, int64, int64) (AdminPromo, error) { return testPromo(), nil }
func (f *fakeAdmin) CreatePromo(_ context.Context, _ int64, in AdminPromoInput) (AdminPromo, error) {
	f.rec("promo.create:" + in.Type + ":" + in.Currency + ":" + in.Code)
	f.created = append(f.created, in)
	return testPromo(), nil
}
func (f *fakeAdmin) SetPromoEnabled(context.Context, int64, int64, bool) (AdminPromo, error) {
	f.rec("promo.enabled")
	return testPromo(), nil
}
func (f *fakeAdmin) DeletePromo(context.Context, int64, int64) error {
	f.rec("promo.delete")
	return nil
}
func (f *fakeAdmin) PromoUses(context.Context, int64, int64, int) ([]AdminPromoUse, bool, error) {
	return []AdminPromoUse{{UserID: 5, User: "Анна <b>", Status: "applied", At: 1789000000, Days: 30},
		{TgID: 77, Status: "refunded", At: 1789000000, Discount: 5000, Currency: "RUB"}}, false, nil
}
func (f *fakeAdmin) Tariff(context.Context, int64, int64) (AdminTariffInfo, error) {
	return AdminTariffInfo{AdminTariff: AdminTariff{ID: 3, Name: "Месяц <b>", Days: 30, Limit: 100 << 30, Devices: 3, PriceRub: 29900, OnSale: true},
		Speed: 100, Terms: 2, Users: 17, Description: "Лучший <i>"}, nil
}
func (f *fakeAdmin) SetTariffOnSale(ctx context.Context, by, id int64, _ bool) (AdminTariffInfo, error) {
	f.rec("tariff.sale")
	return f.Tariff(ctx, by, id)
}
func (f *fakeAdmin) SetTariffPrice(ctx context.Context, by, id int64, cur string, n int64) (AdminTariffInfo, error) {
	f.rec("tariff.price:" + cur)
	f.price = n
	return f.Tariff(ctx, by, id)
}
func (f *fakeAdmin) Tops(_ context.Context, _ int64, kind string) ([]AdminTop, error) {
	u := testUser()
	switch kind {
	case TopShare:
		u.Devices = 5
		return []AdminTop{{AdminUser: u, IPs: 7, Score: 20}}, nil
	case TopMoney:
		return []AdminTop{{AdminUser: u, Value: 150000, Stars: 300}}, nil
	}
	return []AdminTop{{AdminUser: u, Value: 3 << 30}, {AdminUser: AdminUser{ID: 6, Name: "Борис"}, Value: 1 << 30}}, nil
}

func TestShopScreensDraw(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	for _, d := range []string{"aQ:0", "aQ:1", "aq:4", "aq:4:e", "aq:4:d", "aq:4:d:2", "aq:4:u:0", "aW", "aW:d", "aW:t", "aW:p", "aW:r", "aW:s",
		"aW:d:30", "aW:d:30:100", "aW:d:30:100:7", "aW:d:30:100:7:-", "aW:t:10000:1000000:3650:ABCDEFGHIJKLMNOPQRSTUVWXYZ012345",
		"aW:?:v:d", "aW:?:u:d:30", "aW:?:c:d:30:0:7", "aF", "aF:3", "aF:3:s", "aF:3:p", "aF:3:x", "aF:3:P:29990", "aF:3:X:0",
		"aY:t", "aY:m", "aY:s", "a"} {
		text, kb := b.adminScreen(ctx, boss, d, "")
		if text == "" || strings.Contains(text, "<b>SPRING<b>") || strings.Contains(text, "Анна <b>") || strings.Contains(text, "Лучший <i>") {
			t.Errorf("%s: the text is empty or not escaped:\n%s", d, text)
		}
		if ok, why := htmlBalanced(text); !ok {
			t.Errorf("%s: unbalanced markup (%s):\n%s", d, why, text)
		}
		if kb == nil {
			t.Errorf("%s: no keyboard", d)
			continue
		}
		for _, row := range kb.InlineKeyboard {
			if len(row) > 4 {
				t.Errorf("%s: a row of %d buttons", d, len(row))
			}
			for _, bt := range row {
				if len(bt.CallbackData) == 0 || len(bt.CallbackData) > 64 {
					t.Errorf("%s: button %q has data %q", d, bt.Text, bt.CallbackData)
				}
				cmd, _, _ := strings.Cut(bt.CallbackData, ":")
				if _, ok := b.pressKnown(cmd); !ok {
					t.Errorf("%s: button %q leads to %q", d, bt.Text, bt.CallbackData)
				}
			}
		}
	}
}

// Every change of the shop reaches the panel only through its confirmation.
func TestShopChangesNeedConfirmation(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	for _, d := range []string{"aq:4:e", "aq:4:d", "aq:4:d:2", "aF:3:s", "aF:3:P:100", "aW:d:30:0:0:-"} {
		b.adminScreen(ctx, boss, d, "")
	}
	if len(adm.calls) != 0 {
		t.Fatalf("changed before confirmation: %v", adm.calls)
	}
	for _, d := range []string{"aq:4:z:e0", "aq:4:z:d", "aF:3:z:s0", "aF:3:z:P29990", "aF:3:z:X50", "aW:p:20:10:30:-:y", "aW:r:100:0:0:SALE:y"} {
		b.adminScreen(ctx, boss, d, "")
	}
	want := "promo.enabled promo.delete tariff.sale tariff.price:RUB tariff.price:XTR promo.create:percent:: promo.create:fixed:RUB:SALE"
	if got := strings.Join(adm.calls, " "); got != want {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if adm.price != 50 || adm.created[1].Value != 10000 || adm.created[0].Value != 20 || adm.created[0].MaxUses != 10 || adm.created[0].Days != 30 {
		t.Fatalf("values: price %d, created %+v", adm.price, adm.created)
	}
	// A code with what a code may not have is refused before the panel.
	adm.calls = nil
	b.adminScreen(ctx, boss, "aW:d:30:0:0:BAD<b>:y", "")
	if len(adm.calls) != 0 {
		t.Fatalf("a bad code reached the panel: %v", adm.calls)
	}
}

func TestShopTextInput(t *testing.T) {
	for in, want := range map[string]int64{"299": 29900, "299.9": 29990, "299,90 ₽": 29990, "0": 0} {
		if got, ok := parseRub(in); !ok || got != want {
			t.Errorf("parseRub(%q) = %d %v, want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "1.234", "-5", "99999999"} {
		if _, ok := parseRub(in); ok {
			t.Errorf("parseRub(%q) took it", in)
		}
	}
	if cleanCode(" spring 26 ") != "SPRING26" || cleanCode("с1") != "" || cleanCode(strings.Repeat("A", MaxPromoCode+1)) != "" {
		t.Fatal("cleanCode")
	}
}

func TestShopContent(t *testing.T) {
	b := screenBot(&fakeAdmin{user: testUser()})
	ctx := context.Background()
	for d, wants := range map[string][]string{
		"aq:4":            {"SPRING&lt;b&gt;", "+30 дн.", "12 / 100", "▰", "только первая покупка", "выбрано: 2", "Весна &lt;i&gt;"},
		"aF:3":            {"Месяц &lt;b&gt;", "299.00 ₽", "100.0 GB", "100 Мбит/с", "17", "не задана"},
		"aY:s":            {"🌐 7 IP", "📱 5/3", "⚠️ +2"},
		"aY:m":            {"1500.00 ₽ + 300 ⭐", "🥇"},
		"aQ:0":            {"360", "50.0 GB", "Страница"},
		"aW:d:30:100:7:-": {"Дни подписки", "+30", "100", "7 дн.", "случайный"},
	} {
		text, _ := b.adminScreen(ctx, boss, d, "")
		for _, w := range wants {
			if !strings.Contains(text, w) {
				t.Errorf("%s lacks %q:\n%s", d, w, text)
			}
		}
	}
}
