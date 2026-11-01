package botpanel

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/model"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgbot"
)

// The rest of the admin's calls: what the card changes beyond the term and the block, the
// tariffs, and the payments. The same rules as admin.go: check first, one audit entry per
// change, and no secret in what comes back (a reissued link is never returned).

// payStatuses are the statuses a payment list may be narrowed to.
var payStatuses = map[string]bool{"": true, "paid": true, "applied": true, "pending": true, "failed": true, "expired": true, "refunded": true}

// patch applies p to the subscription and names the change in the audit log.
func (a admin) patch(ctx context.Context, by, id int64, p domain.Patch, action string, details map[string]any) (tgbot.AdminUser, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminUser{}, err
	}
	u, err := a.l.Users.Update(ctx, id, p)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return tgbot.AdminUser{}, model.ErrNotFound
		}
		return tgbot.AdminUser{}, err
	}
	a.note(ctx, by, action, id, details)
	return a.view(ctx, u), nil
}

func (a admin) SetTrafficLimit(ctx context.Context, by, id, gb int64) (tgbot.AdminUser, error) {
	if gb < 0 || gb > tgbot.MaxLimitGB {
		if err := a.check(ctx, by); err != nil {
			return tgbot.AdminUser{}, err
		}
		return tgbot.AdminUser{}, model.ErrAdminInput
	}
	p := domain.Patch{}
	if gb == 0 {
		p.ClearTrafficLimit = true
	} else {
		b := gb << 30
		p.TrafficLimit = &b
	}
	return a.patch(ctx, by, id, p, "tgadmin.traffic_limit", map[string]any{"gb": gb})
}

func (a admin) SetDeviceLimit(ctx context.Context, by, id, n int64) (tgbot.AdminUser, error) {
	if n < 0 || n > tgbot.MaxDevices {
		if err := a.check(ctx, by); err != nil {
			return tgbot.AdminUser{}, err
		}
		return tgbot.AdminUser{}, model.ErrAdminInput
	}
	p := domain.Patch{}
	if n == 0 {
		p.ClearDeviceLimit = true
	} else {
		p.DeviceLimit = &n
	}
	return a.patch(ctx, by, id, p, "tgadmin.device_limit", map[string]any{"devices": n})
}

func (a admin) SetNote(ctx context.Context, by, id int64, note string) (tgbot.AdminUser, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > tgbot.MaxNote {
		if err := a.check(ctx, by); err != nil {
			return tgbot.AdminUser{}, err
		}
		return tgbot.AdminUser{}, model.ErrAdminInput
	}
	// The note's text is not kept in the log: its length is.
	return a.patch(ctx, by, id, domain.Patch{Note: &note}, "tgadmin.note", map[string]any{"length": utf8.RuneCountInString(note)})
}

func (a admin) SetTariff(ctx context.Context, by, id, tariff int64) (tgbot.AdminUser, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminUser{}, err
	}
	if tariff <= 0 {
		return tgbot.AdminUser{}, model.ErrAdminInput
	}
	t, err := a.l.Store.Q.GetTariff(ctx, tariff)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tgbot.AdminUser{}, model.ErrNotFound
		}
		return tgbot.AdminUser{}, err
	}
	if t.Archived == 1 {
		return tgbot.AdminUser{}, model.ErrAdminInput
	}
	return a.patch(ctx, by, id, domain.Patch{TariffID: &tariff}, "tgadmin.tariff", map[string]any{"tariff": tariff})
}

func (a admin) ResetTraffic(ctx context.Context, by, id int64) (tgbot.AdminUser, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminUser{}, err
	}
	u, err := a.l.Users.ResetTraffic(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return tgbot.AdminUser{}, model.ErrNotFound
		}
		return tgbot.AdminUser{}, err
	}
	a.note(ctx, by, "tgadmin.reset_traffic", id, nil)
	return a.view(ctx, u), nil
}

func (a admin) Reissue(ctx context.Context, by, id int64) (tgbot.AdminUser, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminUser{}, err
	}
	u, err := a.l.Users.Reissue(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return tgbot.AdminUser{}, model.ErrNotFound
		}
		return tgbot.AdminUser{}, err
	}
	a.note(ctx, by, "tgadmin.reissue", id, nil)
	return a.view(ctx, u), nil
}

func (a admin) Delete(ctx context.Context, by, id int64) error {
	if err := a.check(ctx, by); err != nil {
		return err
	}
	// The name is read first: once deleted there is nothing to name it by.
	u, err := a.l.Store.Q.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrNotFound
		}
		return err
	}
	if err := a.l.Users.Delete(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return model.ErrNotFound
		}
		return err
	}
	a.note(ctx, by, "tgadmin.delete", id, map[string]any{"name": u.Name})
	return nil
}

func (a admin) Tariffs(ctx context.Context, by int64) ([]tgbot.AdminTariff, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, err
	}
	rows, err := a.l.Store.Q.ListTariffs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tgbot.AdminTariff, 0, len(rows))
	for _, t := range rows {
		if t.Archived == 1 {
			continue
		}
		out = append(out, tgbot.AdminTariff{ID: t.ID, Name: t.Name, Days: t.DurationDays, Limit: t.TrafficLimit.Int64,
			Devices: t.DeviceLimit.Int64, PriceRub: t.PriceRub.Int64, OnSale: t.OnSale == 1})
	}
	return out, nil
}

func (a admin) paymentOf(r db.Payment, user string) tgbot.AdminPayment {
	return tgbot.AdminPayment{ID: r.ID, User: user, Provider: r.Provider, Status: r.Status, Amount: r.Amount,
		Currency: r.Currency, At: time.Unix(r.CreatedAt, 0)}
}

// Payments lists a page of payments. The list pages by id, not by a page number, so the page
// asked for is reached by reading past the earlier ones: a page is a handful of rows and the
// admin does not go deep.
func (a admin) Payments(ctx context.Context, by int64, status string, page int) ([]tgbot.AdminPayment, bool, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, false, err
	}
	if !payStatuses[status] {
		return nil, false, model.ErrAdminInput
	}
	if page < 0 {
		page = 0
	}
	if page > 50 {
		return nil, false, model.ErrAdminInput
	}
	before := int64(1 << 62)
	var out []tgbot.AdminPayment
	more := false
	for p := 0; p <= page; p++ {
		rows, err := a.l.Store.Q.ListPayments(ctx, db.ListPaymentsParams{BeforeID: before, Status: status, Lim: tgbot.AdminPageSize + 1})
		if err != nil {
			return nil, false, err
		}
		if len(rows) > tgbot.AdminPageSize {
			more = true
			rows = rows[:tgbot.AdminPageSize]
		} else {
			more = false
		}
		if len(rows) > 0 {
			before = rows[len(rows)-1].Payment.ID
		}
		if p == page {
			for _, r := range rows {
				out = append(out, a.paymentOf(r.Payment, r.UserName))
			}
			break
		}
		if !more {
			return nil, false, nil // fewer pages than asked for
		}
	}
	return out, more, nil
}

func (a admin) Payment(ctx context.Context, by, id int64) (tgbot.AdminPaymentInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminPaymentInfo{}, err
	}
	p, err := a.l.Store.Q.GetPayment(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tgbot.AdminPaymentInfo{}, model.ErrNotFound
		}
		return tgbot.AdminPaymentInfo{}, err
	}
	name := ""
	if p.UserID.Valid {
		if u, err := a.l.Store.Q.GetUser(ctx, p.UserID.Int64); err == nil {
			name = u.Name
		}
	}
	// The provider's own reference, the link to pay and the payload are not handed over: the
	// card says what was bought and how it went.
	return tgbot.AdminPaymentInfo{AdminPayment: a.paymentOf(p, name), Kind: p.Kind, Tariff: p.TariffName,
		TermDays: p.TermDays.Int64, Error: p.Error, PaidAt: p.PaidAt.Int64, Refunded: p.RefundedAt.Valid, UserID: p.UserID.Int64}, nil
}
