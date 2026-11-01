package tgbot

import (
	"context"
	"time"
)

// The admin's side of the bot (/admin). It is a separate interface from Backend and Shop on
// purpose: the subscribers' screens never see it, and the panel serves its methods as
// "Admin.<method>" (botpanel.RPCHandler), after checking on every call that the Telegram
// account named in it is one of Config.AdminIDs. A call from anybody else is refused there,
// whatever the bot's own screens decided to show.

// Limits of what one admin action may do, held by the panel and shown by the screens.
const (
	MaxAdminDays  = 365 // one extension, in days
	AdminPageSize = 8   // rows on a page of a list
)

// AdminOverview is the figures on the admin's first screen.
type AdminOverview struct {
	Users, Active, Expiring, Expired, Disabled int64
	Nodes, NodesOnline                         int64
	PaidMonth                                  int64 // payments applied this month, all currencies
}

// AdminUser is a subscription as the admin's card shows it. It carries no subscription link
// or token: the admin reads who it is and what it has, not how to use it.
type AdminUser struct {
	ID        int64
	Name      string
	Contact   string
	State     string // active, expiring, expired, limited, disabled
	ExpiresAt int64  // unix seconds; 0: no term
	Used      int64  // bytes
	Limit     int64  // bytes; 0: none
	Devices   int64
	TgID      int64 // the linked Telegram account; 0: none
}

// AdminNode is a node as the admin sees it.
type AdminNode struct {
	ID      int64
	Name    string
	Host    string
	Enabled bool
	Online  bool
}

// AdminPayment is a payment in the admin's list.
type AdminPayment struct {
	ID       int64
	User     string
	Provider string
	Status   string
	Amount   int64 // in the currency's minor units
	Currency string
	At       time.Time
}

// Admin is the panel as the admin's screens use it. by is the Telegram account of the admin
// who acts; every method checks it against the list of admins first and fails with
// model.ErrNotAdmin otherwise. Every change is written to the audit log under that account.
type Admin interface {
	Overview(ctx context.Context, by int64) (AdminOverview, error)
	// Users lists the subscriptions whose name or contact holds query (all when empty), a
	// page at a time; the second result is whether there is a page after this one.
	Users(ctx context.Context, by int64, query string, page int) ([]AdminUser, bool, error)
	User(ctx context.Context, by, id int64) (AdminUser, error)
	// GrantDays extends the term by days (1..MaxAdminDays).
	GrantDays(ctx context.Context, by, id, days int64) (AdminUser, error)
	// SetDisabled blocks or unblocks the subscription.
	SetDisabled(ctx context.Context, by, id int64, disabled bool) (AdminUser, error)
	Nodes(ctx context.Context, by int64) ([]AdminNode, error)
	Payments(ctx context.Context, by int64) ([]AdminPayment, error)
}
