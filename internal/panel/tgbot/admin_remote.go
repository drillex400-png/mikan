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

func (a remoteAdmin) UsersBy(ctx context.Context, by int64, filter, query string, page int) ([]AdminUser, bool, error) {
	var (
		us   []AdminUser
		more bool
	)
	err := a.r.call(ctx, "Admin.UsersBy", []any{by, filter, query, page}, &us, &more)
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

func (a remoteAdmin) node(ctx context.Context, method string, args ...any) (AdminNodeInfo, error) {
	var n AdminNodeInfo
	err := a.r.call(ctx, method, args, &n)
	return n, err
}

func (a remoteAdmin) Node(ctx context.Context, by, id int64) (AdminNodeInfo, error) {
	return a.node(ctx, "Admin.Node", by, id)
}

func (a remoteAdmin) SetNodeEnabled(ctx context.Context, by, id int64, on bool) (AdminNodeInfo, error) {
	return a.node(ctx, "Admin.SetNodeEnabled", by, id, on)
}

func (a remoteAdmin) SetNodeField(ctx context.Context, by, id int64, field, value string) (AdminNodeInfo, error) {
	return a.node(ctx, "Admin.SetNodeField", by, id, field, value)
}

func (a remoteAdmin) CheckNode(ctx context.Context, by, id int64) (AdminNodeInfo, error) {
	return a.node(ctx, "Admin.CheckNode", by, id)
}

func (a remoteAdmin) SpeedNode(ctx context.Context, by, id int64) (AdminSpeed, error) {
	var s AdminSpeed
	err := a.r.call(ctx, "Admin.SpeedNode", []any{by, id}, &s)
	return s, err
}

func (a remoteAdmin) UpdateNode(ctx context.Context, by, id int64) (AdminNodeInfo, error) {
	return a.node(ctx, "Admin.UpdateNode", by, id)
}

func (a remoteAdmin) RestartNode(ctx context.Context, by, id int64) error {
	return a.r.call(ctx, "Admin.RestartNode", []any{by, id})
}

func (a remoteAdmin) DeleteNode(ctx context.Context, by, id int64) error {
	return a.r.call(ctx, "Admin.DeleteNode", []any{by, id})
}

func (a remoteAdmin) Payments(ctx context.Context, by int64, status string, page int) ([]AdminPayment, bool, error) {
	var (
		p    []AdminPayment
		more bool
	)
	err := a.r.call(ctx, "Admin.Payments", []any{by, status, page}, &p, &more)
	return p, more, err
}

func (a remoteAdmin) Payment(ctx context.Context, by, id int64) (AdminPaymentInfo, error) {
	var p AdminPaymentInfo
	err := a.r.call(ctx, "Admin.Payment", []any{by, id}, &p)
	return p, err
}

func (a remoteAdmin) user(ctx context.Context, method string, args ...any) (AdminUser, error) {
	var u AdminUser
	err := a.r.call(ctx, method, args, &u)
	return u, err
}

func (a remoteAdmin) SetTrafficLimit(ctx context.Context, by, id, gb int64) (AdminUser, error) {
	return a.user(ctx, "Admin.SetTrafficLimit", by, id, gb)
}

func (a remoteAdmin) SetDeviceLimit(ctx context.Context, by, id, n int64) (AdminUser, error) {
	return a.user(ctx, "Admin.SetDeviceLimit", by, id, n)
}

func (a remoteAdmin) ResetTraffic(ctx context.Context, by, id int64) (AdminUser, error) {
	return a.user(ctx, "Admin.ResetTraffic", by, id)
}

func (a remoteAdmin) Reissue(ctx context.Context, by, id int64) (AdminUser, error) {
	return a.user(ctx, "Admin.Reissue", by, id)
}

func (a remoteAdmin) SetNote(ctx context.Context, by, id int64, note string) (AdminUser, error) {
	return a.user(ctx, "Admin.SetNote", by, id, note)
}

func (a remoteAdmin) SetTariff(ctx context.Context, by, id, tariff int64) (AdminUser, error) {
	return a.user(ctx, "Admin.SetTariff", by, id, tariff)
}

func (a remoteAdmin) Delete(ctx context.Context, by, id int64) error {
	return a.r.call(ctx, "Admin.Delete", []any{by, id})
}

func (a remoteAdmin) Tariffs(ctx context.Context, by int64) ([]AdminTariff, error) {
	var t []AdminTariff
	err := a.r.call(ctx, "Admin.Tariffs", []any{by}, &t)
	return t, err
}

func (a remoteAdmin) NoteGrowth(ctx context.Context, by int64, enabled bool, inviterDays, friendDays int64) error {
	return a.r.call(ctx, "Admin.NoteGrowth", []any{by, enabled, inviterDays, friendDays})
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

func (a remoteAdmin) Promos(ctx context.Context, by int64, page int) ([]AdminPromo, bool, error) {
	var (
		p    []AdminPromo
		more bool
	)
	err := a.r.call(ctx, "Admin.Promos", []any{by, page}, &p, &more)
	return p, more, err
}

func (a remoteAdmin) PromoStats(ctx context.Context, by int64) (AdminPromoStats, error) {
	var s AdminPromoStats
	err := a.r.call(ctx, "Admin.PromoStats", []any{by}, &s)
	return s, err
}

func (a remoteAdmin) Promo(ctx context.Context, by, id int64) (AdminPromo, error) {
	var p AdminPromo
	err := a.r.call(ctx, "Admin.Promo", []any{by, id}, &p)
	return p, err
}

func (a remoteAdmin) CreatePromo(ctx context.Context, by int64, in AdminPromoInput) (AdminPromo, error) {
	var p AdminPromo
	err := a.r.call(ctx, "Admin.CreatePromo", []any{by, in}, &p)
	return p, err
}

func (a remoteAdmin) SetPromoEnabled(ctx context.Context, by, id int64, on bool) (AdminPromo, error) {
	var p AdminPromo
	err := a.r.call(ctx, "Admin.SetPromoEnabled", []any{by, id, on}, &p)
	return p, err
}

func (a remoteAdmin) DeletePromo(ctx context.Context, by, id int64) error {
	return a.r.call(ctx, "Admin.DeletePromo", []any{by, id})
}

func (a remoteAdmin) PromoUses(ctx context.Context, by, id int64, page int) ([]AdminPromoUse, bool, error) {
	var (
		p    []AdminPromoUse
		more bool
	)
	err := a.r.call(ctx, "Admin.PromoUses", []any{by, id, page}, &p, &more)
	return p, more, err
}

func (a remoteAdmin) Tariff(ctx context.Context, by, id int64) (AdminTariffInfo, error) {
	var t AdminTariffInfo
	err := a.r.call(ctx, "Admin.Tariff", []any{by, id}, &t)
	return t, err
}

func (a remoteAdmin) SetTariffOnSale(ctx context.Context, by, id int64, on bool) (AdminTariffInfo, error) {
	var t AdminTariffInfo
	err := a.r.call(ctx, "Admin.SetTariffOnSale", []any{by, id, on}, &t)
	return t, err
}

func (a remoteAdmin) SetTariffPrice(ctx context.Context, by, id int64, currency string, amount int64) (AdminTariffInfo, error) {
	var t AdminTariffInfo
	err := a.r.call(ctx, "Admin.SetTariffPrice", []any{by, id, currency, amount}, &t)
	return t, err
}

func (a remoteAdmin) Tops(ctx context.Context, by int64, kind string) ([]AdminTop, error) {
	var t []AdminTop
	err := a.r.call(ctx, "Admin.Tops", []any{by, kind}, &t)
	return t, err
}
