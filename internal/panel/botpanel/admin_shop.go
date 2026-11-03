package botpanel

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikan/internal/panel/audit"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/model"
	"mikan/internal/panel/promo"
	"mikan/internal/panel/store"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgbot"
)

// The shop from /admin: promo codes, tariffs, and the top lists. Every call checks the admin
// again and every change goes to the audit log with the admin's Telegram ID.

const (
	adminShopPage = 8
	adminTopSize  = 10
)

// noteOn writes a change of something that is not a subscription to the audit log.
func (a admin) noteOn(ctx context.Context, by int64, action, target string, id int64, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	details["actor"] = actor(by)
	_ = audit.Write(ctx, a.l.Store.Q, a.now(), audit.Entry{
		Action: action, TargetType: target, TargetID: strconv.FormatInt(id, 10), Details: details,
	})
}

func promoOf(p db.PromoCode) tgbot.AdminPromo {
	var ids []int64
	_ = json.Unmarshal([]byte(p.TariffIds), &ids)
	return tgbot.AdminPromo{ID: p.ID, Code: p.Code, Name: p.Name, Type: p.Type, Value: p.Value, Currency: p.Currency,
		Uses: p.UsedCount, MaxUses: p.MaxUses.Int64, PerUser: p.PerUserLimit, StartsAt: p.StartsAt.Int64, EndsAt: p.EndsAt.Int64,
		Enabled: p.Enabled != 0, FirstOnly: p.FirstPurchaseOnly != 0, NewOnly: p.NewUsersOnly != 0, Tariffs: len(ids), CreatedAt: p.CreatedAt}
}

func (a admin) Promos(ctx context.Context, by int64, page int) ([]tgbot.AdminPromo, bool, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, false, err
	}
	if page < 0 {
		page = 0
	}
	rows, err := a.l.Store.Q.ListPromoCodes(ctx, db.ListPromoCodesParams{RowOffset: int64(page * adminShopPage), Lim: adminShopPage + 1})
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > adminShopPage
	if more {
		rows = rows[:adminShopPage]
	}
	out := make([]tgbot.AdminPromo, 0, len(rows))
	for _, p := range rows {
		if p.Deleted == 0 {
			out = append(out, promoOf(p))
		}
	}
	return out, more, nil
}

func (a admin) PromoStats(ctx context.Context, by int64) (tgbot.AdminPromoStats, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminPromoStats{}, err
	}
	q := a.l.Store.Q
	codes, err := q.CountPromoCodes(ctx)
	if err != nil {
		return tgbot.AdminPromoStats{}, err
	}
	active, err := q.CountActivePromoCodes(ctx, sql.NullInt64{Int64: a.now().Unix(), Valid: true})
	if err != nil {
		return tgbot.AdminPromoStats{}, err
	}
	st, err := q.PromoStats(ctx)
	if err != nil {
		return tgbot.AdminPromoStats{}, err
	}
	return tgbot.AdminPromoStats{Codes: codes, Active: active, Uses: st.SuccessfulActivations, Days: st.BonusDays,
		Bytes: st.BonusBytes, Orders: st.DiscountOrders}, nil
}

func (a admin) promo(ctx context.Context, id int64) (db.PromoCode, error) {
	p, err := a.l.Store.Q.GetPromoCode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && p.Deleted != 0) {
		return db.PromoCode{}, model.ErrNotFound
	}
	return p, err
}

func (a admin) Promo(ctx context.Context, by, id int64) (tgbot.AdminPromo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminPromo{}, err
	}
	p, err := a.promo(ctx, id)
	if err != nil {
		return tgbot.AdminPromo{}, err
	}
	return promoOf(p), nil
}

// promoAlphabet leaves out the letters and digits that read alike (O and 0, I and 1).
const promoAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newPromoCode() string {
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(promoAlphabet))))
		b[i] = promoAlphabet[n.Int64()]
	}
	return string(b)
}

// cleanPromoCode is the code as it is kept, or "" when it has a character a code may not.
func cleanPromoCode(s string) string {
	c := promo.Normalize(s)
	if c == "" || len(c) > tgbot.MaxPromoCode {
		return ""
	}
	for _, r := range c {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return ""
		}
	}
	return c
}

// promoValueOK holds the bot's bounds of a code's value, narrower than the web panel's.
func promoValueOK(in tgbot.AdminPromoInput) bool {
	switch in.Type {
	case "days":
		return in.Value >= 1 && in.Value <= tgbot.MaxPromoDays && in.Currency == ""
	case "traffic":
		return in.Value >= domain.MinGrantBytes && in.Value <= tgbot.MaxPromoTraffic*domain.GiB && in.Currency == ""
	case "percent":
		return in.Value >= 1 && in.Value <= 100 && (in.Currency == "" || in.Currency == "RUB" || in.Currency == "XTR")
	case "fixed":
		switch in.Currency {
		case "RUB":
			return in.Value >= 100 && in.Value <= tgbot.MaxPriceRub*100
		case "XTR":
			return in.Value >= 1 && in.Value <= tgbot.MaxPriceStars
		}
	}
	return false
}

func (a admin) CreatePromo(ctx context.Context, by int64, in tgbot.AdminPromoInput) (tgbot.AdminPromo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminPromo{}, err
	}
	if !promoValueOK(in) || in.MaxUses < 0 || in.MaxUses > tgbot.MaxPromoUses || in.Days < 0 || in.Days > tgbot.MaxPromoDays {
		return tgbot.AdminPromo{}, model.ErrAdminInput
	}
	code, picked := cleanPromoCode(in.Code), strings.TrimSpace(in.Code) == ""
	if picked {
		code = newPromoCode()
	} else if code == "" {
		return tgbot.AdminPromo{}, model.ErrAdminInput
	}
	now := a.now()
	p := db.CreatePromoCodeParams{Code: code, Name: code, Type: in.Type, Value: in.Value, Currency: in.Currency,
		PerUserLimit: 1, TariffIds: "[]", Enabled: 1, CreatedAt: now.Unix()}
	if in.MaxUses > 0 {
		p.MaxUses = sql.NullInt64{Int64: in.MaxUses, Valid: true}
	}
	if in.Days > 0 {
		p.EndsAt = sql.NullInt64{Int64: now.Add(time.Duration(in.Days) * 24 * time.Hour).Unix(), Valid: true}
	}
	var x db.PromoCode
	var err error
	for try := 0; ; try++ {
		x, err = a.l.Store.Q.CreatePromoCode(ctx, p)
		if !store.IsUnique(err) || !picked || try == 3 {
			break
		}
		p.Code = newPromoCode()
		p.Name = p.Code
	}
	if store.IsUnique(err) {
		return tgbot.AdminPromo{}, model.ErrAdminInput // the code is taken
	}
	if err != nil {
		return tgbot.AdminPromo{}, err
	}
	a.noteOn(ctx, by, "tgadmin.promo_create", "promocode", x.ID, map[string]any{"code": x.Code, "type": x.Type, "value": x.Value})
	return promoOf(x), nil
}

func (a admin) SetPromoEnabled(ctx context.Context, by, id int64, on bool) (tgbot.AdminPromo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminPromo{}, err
	}
	if _, err := a.promo(ctx, id); err != nil {
		return tgbot.AdminPromo{}, err
	}
	if _, err := a.l.Store.Q.SetPromoEnabled(ctx, db.SetPromoEnabledParams{Enabled: domain.Flag(on), ID: id}); err != nil {
		return tgbot.AdminPromo{}, err
	}
	p, err := a.promo(ctx, id)
	if err != nil {
		return tgbot.AdminPromo{}, err
	}
	a.noteOn(ctx, by, "tgadmin.promo_enabled", "promocode", id, map[string]any{"code": p.Code, "enabled": on})
	return promoOf(p), nil
}

func (a admin) DeletePromo(ctx context.Context, by, id int64) error {
	if err := a.check(ctx, by); err != nil {
		return err
	}
	p, err := a.promo(ctx, id)
	if err != nil {
		return err
	}
	n, err := a.l.Store.Q.DeletePromoCode(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return model.ErrNotFound
	}
	a.noteOn(ctx, by, "tgadmin.promo_delete", "promocode", id, map[string]any{"code": p.Code})
	return nil
}

func (a admin) PromoUses(ctx context.Context, by, id int64, page int) ([]tgbot.AdminPromoUse, bool, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, false, err
	}
	p, err := a.promo(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if page < 0 {
		page = 0
	}
	// The list finds by a part of the code, so a longer code with this one in it comes too.
	var mine []db.ListPromoRedemptionsWithCodeRow
	const batch = 200
	for off := int64(0); len(mine) <= (page+1)*adminShopPage; off += batch {
		rows, err := a.l.Store.Q.ListPromoRedemptionsWithCode(ctx, db.ListPromoRedemptionsWithCodeParams{Q: p.Code, RowOffset: off, Lim: batch})
		if err != nil {
			return nil, false, err
		}
		for _, r := range rows {
			if r.PromoID == id {
				mine = append(mine, r)
			}
		}
		if len(rows) < batch {
			break
		}
	}
	from := page * adminShopPage
	if from >= len(mine) {
		return []tgbot.AdminPromoUse{}, false, nil
	}
	to := min(from+adminShopPage, len(mine))
	out := make([]tgbot.AdminPromoUse, 0, to-from)
	for _, r := range mine[from:to] {
		u := tgbot.AdminPromoUse{UserID: r.UserID.Int64, TgID: r.TgID, Status: r.Status, At: r.RedeemedAt, Days: r.Days,
			Bytes: r.Bytes, Discount: r.DiscountAmount, Currency: r.Currency}
		if r.UserID.Valid {
			if us, err := a.l.Store.Q.GetUser(ctx, r.UserID.Int64); err == nil {
				u.User = us.Name
			}
		}
		out = append(out, u)
	}
	return out, len(mine) > to, nil
}

func (a admin) tariffInfo(ctx context.Context, t db.Tariff) (tgbot.AdminTariffInfo, error) {
	info := tgbot.AdminTariffInfo{AdminTariff: tgbot.AdminTariff{ID: t.ID, Name: t.Name, Days: t.DurationDays, Limit: t.TrafficLimit.Int64,
		Devices: t.DeviceLimit.Int64, PriceRub: t.PriceRub.Int64, PriceStars: t.PriceStars.Int64, OnSale: t.OnSale == 1},
		Speed: t.SpeedLimit.Int64, Description: t.Description}
	terms, err := a.l.Store.Q.ListTariffTerms(ctx, t.ID)
	if err != nil {
		return info, err
	}
	if len(terms) > 1 {
		info.Terms = len(terms) - 1
	}
	if err := a.l.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE tariff_id = $1", t.ID).Scan(&info.Users); err != nil {
		return info, err
	}
	return info, nil
}

func (a admin) tariff(ctx context.Context, id int64) (db.Tariff, error) {
	t, err := a.l.Store.Q.GetTariff(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && t.Archived == 1) {
		return db.Tariff{}, model.ErrNotFound
	}
	return t, err
}

func (a admin) Tariff(ctx context.Context, by, id int64) (tgbot.AdminTariffInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminTariffInfo{}, err
	}
	t, err := a.tariff(ctx, id)
	if err != nil {
		return tgbot.AdminTariffInfo{}, err
	}
	return a.tariffInfo(ctx, t)
}

// setTariff writes the tariff with its first term changed by f, everything else as it was.
func (a admin) setTariff(ctx context.Context, id int64, f func(t *db.Tariff, first *domain.Term)) (db.Tariff, error) {
	var out db.Tariff
	err := a.l.Store.TxRC(ctx, func(q *db.Queries) error {
		t, err := q.GetTariff(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && t.Archived == 1) {
			return model.ErrNotFound
		}
		if err != nil {
			return err
		}
		rows, err := q.ListTariffTerms(ctx, id)
		if err != nil {
			return err
		}
		first := domain.Term{Days: t.DurationDays, PriceStars: t.PriceStars, PriceRub: t.PriceRub}
		f(&t, &first)
		t.PriceStars, t.PriceRub = first.PriceStars, first.PriceRub
		out, err = q.UpdateTariff(ctx, db.UpdateTariffParams{Name: t.Name, TrafficLimit: t.TrafficLimit, DurationDays: t.DurationDays,
			DeviceLimit: t.DeviceLimit, ResetStrategy: t.ResetStrategy, PriceLabel: t.PriceLabel, Sort: t.Sort, BillingDay: t.BillingDay,
			PriceStars: t.PriceStars, PriceRub: t.PriceRub, OnSale: t.OnSale, ID: t.ID, SpeedLimit: t.SpeedLimit, Description: t.Description})
		if err != nil || len(rows) < 2 {
			return err
		}
		terms := []domain.Term{first}
		for _, r := range rows[1:] {
			terms = append(terms, domain.Term{Days: r.Days, PriceStars: r.PriceStars, PriceRub: r.PriceRub})
		}
		return domain.SetTariffTerms(ctx, q, id, terms)
	})
	return out, err
}

func (a admin) SetTariffOnSale(ctx context.Context, by, id int64, on bool) (tgbot.AdminTariffInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminTariffInfo{}, err
	}
	t, err := a.setTariff(ctx, id, func(t *db.Tariff, _ *domain.Term) { t.OnSale = domain.Flag(on) })
	if err != nil {
		return tgbot.AdminTariffInfo{}, err
	}
	a.noteOn(ctx, by, "tgadmin.tariff_sale", "tariff", id, map[string]any{"on_sale": on})
	return a.tariffInfo(ctx, t)
}

func (a admin) SetTariffPrice(ctx context.Context, by, id int64, currency string, amount int64) (tgbot.AdminTariffInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminTariffInfo{}, err
	}
	price := sql.NullInt64{Int64: amount, Valid: amount > 0}
	switch {
	case currency == "RUB" && amount >= 0 && amount <= tgbot.MaxPriceRub*100:
	case currency == "XTR" && amount >= 0 && amount <= tgbot.MaxPriceStars:
	default:
		return tgbot.AdminTariffInfo{}, model.ErrAdminInput
	}
	t, err := a.setTariff(ctx, id, func(_ *db.Tariff, first *domain.Term) {
		if currency == "RUB" {
			first.PriceRub = price
		} else {
			first.PriceStars = price
		}
	})
	if err != nil {
		return tgbot.AdminTariffInfo{}, err
	}
	a.noteOn(ctx, by, "tgadmin.tariff_price", "tariff", id, map[string]any{"currency": currency, "amount": amount})
	return a.tariffInfo(ctx, t)
}

// The signs a subscription is shared: its devices came from more addresses in the last day than
// one person has (shareIPs), or it has more devices than its limit.
const (
	shareWindow = 24 * time.Hour
	shareIPs    = 3
)

func (a admin) Tops(ctx context.Context, by int64, kind string) ([]tgbot.AdminTop, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, err
	}
	type cand struct{ id, value, stars, devices, ips int64 }
	var q string
	var args []any
	switch kind {
	case tgbot.TopTraffic:
		q = "SELECT id, used_up + used_down, 0, 0, 0 FROM users WHERE used_up + used_down > 0 ORDER BY used_up + used_down DESC, id LIMIT $1"
		args = []any{adminTopSize}
	case tgbot.TopMoney:
		q = `SELECT user_id, COALESCE(sum(amount) FILTER (WHERE currency = 'RUB'), 0), COALESCE(sum(amount) FILTER (WHERE currency = 'XTR'), 0), 0, 0
FROM payments WHERE status = 'applied' AND user_id IS NOT NULL AND user_id IN (SELECT id FROM users)
GROUP BY user_id ORDER BY 2 DESC, 3 DESC, user_id LIMIT $1`
		args = []any{adminTopSize}
	case tgbot.TopShare:
		q = `SELECT user_id, 0, 0, count(*), count(DISTINCT last_ip) FILTER (WHERE last_seen >= $1 AND last_ip <> '')
FROM bound_devices GROUP BY user_id
HAVING count(DISTINCT last_ip) FILTER (WHERE last_seen >= $1 AND last_ip <> '') >= $2 OR count(*) > 1
ORDER BY 5 DESC, 4 DESC, user_id LIMIT 200`
		args = []any{a.now().Add(-shareWindow).Unix(), shareIPs}
	default:
		return nil, model.ErrAdminInput
	}
	rows, err := a.l.Store.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var cs []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.value, &c.stars, &c.devices, &c.ips); err != nil {
			rows.Close()
			return nil, err
		}
		cs = append(cs, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []tgbot.AdminTop{}
	for _, c := range cs {
		u, err := a.l.Store.Q.GetUser(ctx, c.id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		t := tgbot.AdminTop{AdminUser: a.view(ctx, u), Value: c.value, Stars: c.stars, IPs: c.ips}
		if kind == tgbot.TopShare {
			over := int64(0)
			if t.DeviceLimit > 0 && t.Devices > t.DeviceLimit {
				over = t.Devices - t.DeviceLimit
			}
			if c.ips < shareIPs && over == 0 {
				continue
			}
			t.Score = c.ips*2 + over*3
		}
		out = append(out, t)
	}
	if kind == tgbot.TopShare {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
		if len(out) > adminTopSize {
			out = out[:adminTopSize]
		}
	}
	return out, nil
}
