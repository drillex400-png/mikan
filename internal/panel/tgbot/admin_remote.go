package tgbot

import "context"

// Admin is the admin's side over the Panel API ("Admin.<method>"), the same way Shop is.
func (r *Remote) Admin() Admin { return remoteAdmin{r} }

type remoteAdmin struct{ r *Remote }

var _ Admin = remoteAdmin{}

func (a remoteAdmin) Overview(ctx context.Context, by int64) (AdminOverview, error) {
	var o AdminOverview
	err := a.r.call(ctx, "Admin.Overview", []any{by}, &o)
	return o, err
}

func (a remoteAdmin) Users(ctx context.Context, by int64, query string, page int) ([]AdminUser, bool, error) {
	var (
		us   []AdminUser
		more bool
	)
	err := a.r.call(ctx, "Admin.Users", []any{by, query, page}, &us, &more)
	return us, more, err
}

func (a remoteAdmin) User(ctx context.Context, by, id int64) (AdminUser, error) {
	var u AdminUser
	err := a.r.call(ctx, "Admin.User", []any{by, id}, &u)
	return u, err
}

func (a remoteAdmin) GrantDays(ctx context.Context, by, id, days int64) (AdminUser, error) {
	var u AdminUser
	err := a.r.call(ctx, "Admin.GrantDays", []any{by, id, days}, &u)
	return u, err
}

func (a remoteAdmin) SetDisabled(ctx context.Context, by, id int64, disabled bool) (AdminUser, error) {
	var u AdminUser
	err := a.r.call(ctx, "Admin.SetDisabled", []any{by, id, disabled}, &u)
	return u, err
}

func (a remoteAdmin) Nodes(ctx context.Context, by int64) ([]AdminNode, error) {
	var n []AdminNode
	err := a.r.call(ctx, "Admin.Nodes", []any{by}, &n)
	return n, err
}

func (a remoteAdmin) Payments(ctx context.Context, by int64) ([]AdminPayment, error) {
	var p []AdminPayment
	err := a.r.call(ctx, "Admin.Payments", []any{by}, &p)
	return p, err
}

func (a remoteAdmin) BroadcastAudience(ctx context.Context, by int64) (int, error) {
	var n int
	err := a.r.call(ctx, "Admin.BroadcastAudience", []any{by}, &n)
	return n, err
}

func (a remoteAdmin) Broadcast(ctx context.Context, by int64, text string) (int, error) {
	var n int
	err := a.r.call(ctx, "Admin.Broadcast", []any{by, text}, &n)
	return n, err
}
