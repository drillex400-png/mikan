package botpanel

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mikan/internal/panel/audit"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/model"
	"mikan/internal/panel/nodeupdate"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgate"
	"mikan/internal/panel/tgbot"
)

// The panel's side of /admin. Nothing here trusts the bot: every method opens with check, so
// a call from an account that is not in the admin list is refused whatever the bot decided to
// show, and every change is written to the audit log with the Telegram account that made it.

// Admin is tgbot.Admin inside the panel's process.
func (l *Local) Admin() tgbot.Admin { return admin{l} }

type admin struct{ l *Local }

// actor is how the audit log names the admin: "tg:<id>".
func actor(by int64) string { return "tg:" + strconv.FormatInt(by, 10) }

// check: by is an admin right now (the list is read again, so a removed admin loses access at
// once).
func (a admin) check(ctx context.Context, by int64) error {
	if !a.l.Config(ctx).IsAdmin(by) {
		return model.ErrNotAdmin
	}
	return nil
}

func (a admin) now() time.Time {
	if a.l.Now != nil {
		return a.l.Now()
	}
	return time.Now()
}

// note writes what an admin changed. It carries no secret: ids and numbers only.
func (a admin) note(ctx context.Context, by int64, action string, id int64, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	details["actor"] = actor(by)
	_ = audit.Write(ctx, a.l.Store.Q, a.now(), audit.Entry{
		Action: action, TargetType: "user", TargetID: strconv.FormatInt(id, 10), Details: details,
	})
}

func (a admin) Overview(ctx context.Context, by int64) (tgbot.AdminOverview, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminOverview{}, err
	}
	now := a.now()
	c, err := domain.CountStates(ctx, a.l.Store.Q, now)
	if err != nil {
		return tgbot.AdminOverview{}, err
	}
	o := tgbot.AdminOverview{Users: c.Total, Active: c.Active, Expiring: c.Expiring, Expired: c.Expired, Disabled: c.Disabled}
	nodes, err := a.l.Store.Q.ListNodes(ctx)
	if err != nil {
		return o, err
	}
	for _, n := range nodes {
		o.Nodes++
		if n.Enabled == 1 {
			o.NodesOnline++
		}
	}
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	if rows, err := a.l.Store.Q.PaymentTotals(ctx, sql.NullInt64{Int64: monthStart, Valid: true}); err == nil {
		for _, r := range rows {
			o.PaidMonth += r.N
		}
	}
	o.Revenue = a.revenue(ctx, now)
	o.ExpireSoon = c.Expiring
	if users, err := a.l.Store.Q.ListUsers(ctx); err == nil {
		since := now.Add(-24 * time.Hour).Unix()
		for _, u := range users {
			if u.CreatedAt >= since {
				o.NewToday++
			}
		}
	}
	return o, nil
}

func (a admin) view(ctx context.Context, u db.User) tgbot.AdminUser {
	now := a.now()
	grants := int64(0)
	if g, err := a.l.GrantsLeft(ctx, u.ID, now); err == nil {
		grants = g.Main
	}
	v := tgbot.AdminUser{
		ID: u.ID, Name: u.Name, Contact: u.Contact, State: domain.State(u, grants, now),
		Used: u.UsedUp + u.UsedDown, Limit: u.TrafficLimit.Int64, ExpiresAt: u.ExpiresAt.Int64,
	}
	if n, err := a.l.DeviceCount(ctx, u.ID); err == nil {
		v.Devices = n
	}
	v.DeviceLimit, v.Note, v.CreatedAt = u.DeviceLimit.Int64, u.Note, u.CreatedAt
	if u.TariffID.Valid {
		if t, err := a.l.Store.Q.GetTariff(ctx, u.TariffID.Int64); err == nil {
			v.Tariff = t.Name
		}
	}
	if tg, err := a.l.Owner(ctx, u.ID); err == nil {
		v.TgID = tg
	}
	return v
}

func (a admin) Users(ctx context.Context, by int64, query string, page int) ([]tgbot.AdminUser, bool, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, false, err
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
	var out []tgbot.AdminUser
	skip, more := page*tgbot.AdminPageSize, false
	for _, u := range all {
		if q != "" && !strings.Contains(strings.ToLower(u.Name), q) && !strings.Contains(strings.ToLower(u.Contact), q) {
			continue
		}
		if skip > 0 {
			skip--
			continue
		}
		if len(out) == tgbot.AdminPageSize {
			more = true
			break
		}
		out = append(out, a.view(ctx, u))
	}
	return out, more, nil
}

func (a admin) User(ctx context.Context, by, id int64) (tgbot.AdminUser, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminUser{}, err
	}
	u, err := a.l.Store.Q.GetUser(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return tgbot.AdminUser{}, model.ErrNotFound
		}
		return tgbot.AdminUser{}, err
	}
	return a.view(ctx, u), nil
}

func (a admin) GrantDays(ctx context.Context, by, id, days int64) (tgbot.AdminUser, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminUser{}, err
	}
	if days < 1 || days > tgbot.MaxAdminDays {
		return tgbot.AdminUser{}, model.ErrAdminInput
	}
	u, err := a.l.Users.Extend(ctx, id, days)
	if err != nil {
		if err == sql.ErrNoRows {
			return tgbot.AdminUser{}, model.ErrNotFound
		}
		return tgbot.AdminUser{}, err
	}
	a.note(ctx, by, "tgadmin.extend", id, map[string]any{"days": days})
	return a.view(ctx, u), nil
}

func (a admin) SetDisabled(ctx context.Context, by, id int64, disabled bool) (tgbot.AdminUser, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminUser{}, err
	}
	u, err := a.l.Users.Update(ctx, id, domain.Patch{Disabled: &disabled})
	if err != nil {
		if err == sql.ErrNoRows {
			return tgbot.AdminUser{}, model.ErrNotFound
		}
		return tgbot.AdminUser{}, err
	}
	a.note(ctx, by, "tgadmin.disable", id, map[string]any{"disabled": disabled})
	return a.view(ctx, u), nil
}

func (a admin) Nodes(ctx context.Context, by int64) ([]tgbot.AdminNode, error) {
	if err := a.check(ctx, by); err != nil {
		return nil, err
	}
	rows, err := a.l.Store.Q.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tgbot.AdminNode, 0, len(rows))
	for _, n := range rows {
		host := n.PublicHost
		if host == "" {
			host = n.Domain
		}
		// The list shows a server the way its card does: the panel's own node is up while the
		// panel answers, the others as the node manager last found them.
		item := tgbot.AdminNode{ID: n.ID, Name: n.Name, Host: host, Enabled: n.Enabled == 1, Local: nodeupdate.Local(n)}
		if item.Local {
			item.Online = true
		} else if a.l.Nodes != nil {
			if hv, ok := a.l.Nodes.Health(n.ID); ok && !hv.CheckedAt.IsZero() && hv.OK {
				item.Online, item.Version = true, hv.Health.Version
				item.Behind = a.l.Version != "" && nodeupdate.Behind(a.l.Version, item.Version)
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (a admin) BroadcastAudience(ctx context.Context, by int64) (int, error) {
	if err := a.check(ctx, by); err != nil {
		return 0, err
	}
	targets, err := a.l.BroadcastTargets(ctx)
	return len(targets), err
}

// NoteGrowth records a change of the referral programme's settings: numbers and a flag only.
func (a admin) NoteGrowth(ctx context.Context, by int64, enabled bool, inviterDays, friendDays int64) error {
	if err := a.check(ctx, by); err != nil {
		return err
	}
	if inviterDays < 0 || friendDays < 0 || inviterDays > 365 || friendDays > 365 {
		return model.ErrAdminInput
	}
	return audit.Write(ctx, a.l.Store.Q, a.now(), audit.Entry{
		Action: "growth.settings", TargetType: "addon", TargetID: "mikan-bot",
		Details: map[string]any{"actor": actor(by), "enabled": enabled, "inviter_days": inviterDays, "friend_days": friendDays},
	})
}

// Broadcast sends through the same path as the admin panel's broadcast, so the rate limit,
// the one-at-a-time rule and the progress are the same. The text is checked here too: the bot
// has checked it, but the panel does not rely on that.
func (a admin) Broadcast(ctx context.Context, by int64, text string) (int, error) {
	if err := a.check(ctx, by); err != nil {
		return 0, err
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > tgbot.MaxBroadcast {
		return 0, model.ErrAdminInput
	}
	if a.l.Broadcast == nil {
		return 0, model.ErrBroadcastOff
	}
	n, err := a.l.Broadcast(ctx, text)
	switch {
	case errors.Is(err, tgate.ErrOff):
		return 0, model.ErrBroadcastOff
	case errors.Is(err, ErrBusy):
		return 0, model.ErrBroadcastBusy
	case err != nil:
		return 0, err
	}
	// The text itself is not kept in the log: its length and who it reached are.
	_ = audit.Write(ctx, a.l.Store.Q, a.now(), audit.Entry{Action: "tgadmin.broadcast",
		Details: map[string]any{"actor": actor(by), "recipients": n, "length": utf8.RuneCountInString(text)}})
	return n, nil
}
