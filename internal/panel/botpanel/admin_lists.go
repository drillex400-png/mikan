package botpanel

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/model"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgbot"
)

// The admin's lists and figures that go beyond the first screens: the clients narrowed by a
// filter, and the money by period. They only read; nothing here changes a subscription.

// lowData is the share of the traffic limit from which a client counts as nearly out.
const lowData = 90

// matches says whether u passes the filter. state is domain.State of u.
func matches(filter string, u db.User, state string) bool {
	switch filter {
	case tgbot.FilterSoon:
		return state == domain.StateExpiring
	case tgbot.FilterEnded:
		return state == domain.StateExpired
	case tgbot.FilterBlocked:
		return state == domain.StateDisabled
	case tgbot.FilterLowData:
		lim := u.TrafficLimit.Int64
		return u.TrafficLimit.Valid && lim > 0 && (u.UsedUp+u.UsedDown)*100/lim >= lowData
	}
	return true
}

// UsersBy is Users narrowed by a filter. The ones whose term ends soonest come first for the
// filters about the term, so the page the admin opens is the page that needs him.
func (a admin) UsersBy(ctx context.Context, by int64, filter, query string, page int) ([]tgbot.AdminUser, bool, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, false, err
	}
	switch filter {
	case tgbot.FilterAll, tgbot.FilterSoon, tgbot.FilterEnded, tgbot.FilterLowData, tgbot.FilterNoTG, tgbot.FilterBlocked:
	default:
		return nil, false, model.ErrAdminInput
	}
	all, err := a.l.Store.Q.ListUsers(ctx)
	if err != nil {
		return nil, false, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if len(q) > 64 {
		q = q[:64]
	}
	if page < 0 {
		page = 0
	}
	now := a.now()
	var kept []db.User
	for _, u := range all {
		if q != "" && !strings.Contains(strings.ToLower(u.Name), q) && !strings.Contains(strings.ToLower(u.Contact), q) {
			continue
		}
		grants := int64(0)
		if g, err := a.l.GrantsLeft(ctx, u.ID, now); err == nil {
			grants = g.Main
		}
		if !matches(filter, u, domain.State(u, grants, now)) {
			continue
		}
		if filter == tgbot.FilterNoTG {
			if tg, err := a.l.Owner(ctx, u.ID); err == nil && tg != 0 {
				continue
			}
		}
		kept = append(kept, u)
	}
	if filter == tgbot.FilterSoon || filter == tgbot.FilterEnded {
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].ExpiresAt.Int64 < kept[j].ExpiresAt.Int64 })
	}
	start := page * tgbot.AdminPageSize
	if start >= len(kept) {
		return nil, false, nil
	}
	end := start + tgbot.AdminPageSize
	more := end < len(kept)
	if end > len(kept) {
		end = len(kept)
	}
	out := make([]tgbot.AdminUser, 0, end-start)
	for _, u := range kept[start:end] {
		out = append(out, a.view(ctx, u))
	}
	return out, more, nil
}

// revenue is what was paid since three moments, by currency.
func (a admin) revenue(ctx context.Context, now time.Time) []tgbot.AdminMoney {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	week := now.Add(-7 * 24 * time.Hour).Unix()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	by := map[string]*tgbot.AdminMoney{}
	for i, since := range []int64{day, week, month} {
		rows, err := a.l.Store.Q.PaymentTotals(ctx, sql.NullInt64{Int64: since, Valid: true})
		if err != nil {
			continue
		}
		for _, r := range rows {
			m := by[r.Currency]
			if m == nil {
				m = &tgbot.AdminMoney{Currency: r.Currency}
				by[r.Currency] = m
			}
			switch i {
			case 0:
				m.Today = r.Total
			case 1:
				m.Week = r.Total
			case 2:
				m.Month = r.Total
			}
		}
	}
	out := make([]tgbot.AdminMoney, 0, len(by))
	for _, m := range by {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}
