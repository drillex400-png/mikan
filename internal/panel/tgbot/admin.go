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
	MaxAdminDays  = 365    // one extension, in days
	AdminPageSize = 8      // rows on a page of a list
	MaxBroadcast  = 3000   // characters in a broadcast, as in the admin panel
	MaxLimitGB    = 100000 // one traffic limit, in GB
	MaxDevices    = 1000   // one device limit
	MaxNote       = 500    // characters in a note
)

// AdminOverview is the figures on the admin's first screen.
type AdminOverview struct {
	Users, Active, Expiring, Expired, Disabled int64
	Nodes, NodesOnline                         int64
	PaidMonth                                  int64 // payments applied this month, all currencies
	// What an older panel does not send is zero and the screens leave it out. Money is per
	// currency, never added up across them.
	NewToday   int64        // subscriptions made in the last 24 hours
	ExpireSoon int64        // terms ending within 3 days
	Revenue    []AdminMoney // by currency: today, 7 days, this month
}

// AdminMoney is what was paid in one currency, in its minor units.
type AdminMoney struct {
	Currency           string
	Today, Week, Month int64
}

// The filters of the list of clients (UsersBy): a letter in the bot's buttons.
const (
	FilterAll     = ""
	FilterSoon    = "s" // the term ends within the expiring window
	FilterEnded   = "e" // the term has ended
	FilterLowData = "t" // 90% of the traffic or more used
	FilterNoTG    = "n" // no Telegram account linked
	FilterBlocked = "b" // blocked by the admin
)

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
	// What the card also shows. DeviceLimit and Tariff are 0/"" when there is none.
	DeviceLimit int64
	Tariff      string
	Note        string
	CreatedAt   int64 // unix seconds
	Balance     int64 // in the minor units of the bot's currency; 0: none
}

// AdminTariff is a tariff as the admin picks it.
type AdminTariff struct {
	ID       int64
	Name     string
	Days     int64
	Limit    int64 // bytes; 0: none
	Devices  int64 // 0: none
	PriceRub int64 // kopecks; 0: none
	OnSale   bool
	// PriceStars is the price in Stars; 0: none.
	PriceStars int64
}

// AdminPaymentInfo is one payment as its card shows it.
type AdminPaymentInfo struct {
	AdminPayment
	Kind     string
	Tariff   string
	TermDays int64
	Error    string
	PaidAt   int64
	Refunded bool
	UserID   int64
}

// AdminNode is a node as the admin sees it.
type AdminNode struct {
	ID      int64
	Name    string
	Host    string
	Enabled bool
	Online  bool
	// What an older panel does not send is zero and the screens leave it out.
	Local   bool // the panel's own node
	Version string
	Behind  bool // runs an older version than the panel
}

// AdminNodeInfo is one server's card: how it is, not how to reach it by key. It carries no
// node key, certificate or API address.
type AdminNodeInfo struct {
	AdminNode
	PublicName  string
	Domain      string
	Status      string // ok, error, unknown
	Error       string // why the panel cannot reach it, as the panel words it
	ErrorSince  int64  // unix seconds; 0: unknown
	Inbounds    int
	Listeners   int
	ListenersOK int
	Conns       int
	CPU         float64 // percent of the whole server
	MemUsed     uint64
	MemTotal    uint64
	Traffic24h  int64 // bytes, both ways
	ChannelMbps int64 // 0: not set
	CanUpdate   bool
	Updating    bool // an update of it is running
	PanelVer    string
}

// AdminSpeed is a speed test of a node.
type AdminSpeed struct {
	PingMs  float64
	LossPct float64
	DownBps int64
	UpBps   int64
	Error   string
}

// The fields of a server the bot may change (SetNodeField): name and public name are text, the
// channel is Mbit/s (0 clears it).
const (
	NodeFieldName    = "n"
	NodeFieldPublic  = "p"
	NodeFieldChannel = "c"
)

// MaxNodeText is the longest name of a server the bot sets.
const MaxNodeText = 80

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
	// UsersBy is Users narrowed by a filter (Filter*), soonest-ending first for the term ones.
	UsersBy(ctx context.Context, by int64, filter, query string, page int) ([]AdminUser, bool, error)
	// GrantDays extends the term by days (1..MaxAdminDays).
	GrantDays(ctx context.Context, by, id, days int64) (AdminUser, error)
	// SetDisabled blocks or unblocks the subscription.
	SetDisabled(ctx context.Context, by, id int64, disabled bool) (AdminUser, error)
	// SetTrafficLimit sets the traffic limit in GB (0: no limit).
	SetTrafficLimit(ctx context.Context, by, id, gb int64) (AdminUser, error)
	// SetDeviceLimit sets the device limit (0: no limit).
	SetDeviceLimit(ctx context.Context, by, id, n int64) (AdminUser, error)
	// ResetTraffic starts the counted traffic over.
	ResetTraffic(ctx context.Context, by, id int64) (AdminUser, error)
	// Reissue gives the subscription a new link; the old one stops working. The link is not
	// returned: the admin never sees it.
	Reissue(ctx context.Context, by, id int64) (AdminUser, error)
	// SetNote sets the admin's note on the subscription (empty clears it).
	SetNote(ctx context.Context, by, id int64, note string) (AdminUser, error)
	// Delete removes the subscription for good.
	Delete(ctx context.Context, by, id int64) error
	// Tariffs lists the tariffs that are not archived.
	Tariffs(ctx context.Context, by int64) ([]AdminTariff, error)
	// SetTariff applies a tariff to the subscription: its limits, and the term starts over.
	SetTariff(ctx context.Context, by, id, tariff int64) (AdminUser, error)
	Nodes(ctx context.Context, by int64) ([]AdminNode, error)
	// Node is one server's card.
	Node(ctx context.Context, by, id int64) (AdminNodeInfo, error)
	// SetNodeEnabled turns the server on or off for the subscriptions.
	SetNodeEnabled(ctx context.Context, by, id int64, on bool) (AdminNodeInfo, error)
	// SetNodeField changes the name, the public name or the channel (NodeField*).
	SetNodeField(ctx context.Context, by, id int64, field, value string) (AdminNodeInfo, error)
	// CheckNode asks the server for its state at once.
	CheckNode(ctx context.Context, by, id int64) (AdminNodeInfo, error)
	// SpeedNode measures the server's own way to the internet.
	SpeedNode(ctx context.Context, by, id int64) (AdminSpeed, error)
	// UpdateNode asks the server to update itself to the panel's version.
	UpdateNode(ctx context.Context, by, id int64) (AdminNodeInfo, error)
	// RestartNode restarts the server's containers.
	RestartNode(ctx context.Context, by, id int64) error
	// DeleteNode removes the server; what went out through it goes another way.
	DeleteNode(ctx context.Context, by, id int64) error
	// Payments lists a page of payments, newest first; status filters ("" all, paid, pending,
	// failed, refunded). The second result is whether there is a page after this one.
	Payments(ctx context.Context, by int64, status string, page int) ([]AdminPayment, bool, error)
	// Payment is one payment's card.
	Payment(ctx context.Context, by, id int64) (AdminPaymentInfo, error)
	// Promos lists a page of promo codes, newest first; the second result: a page after it.
	Promos(ctx context.Context, by int64, page int) ([]AdminPromo, bool, error)
	PromoStats(ctx context.Context, by int64) (AdminPromoStats, error)
	Promo(ctx context.Context, by, id int64) (AdminPromo, error)
	// CreatePromo makes a promo code; a code that is taken is ErrAdminInput.
	CreatePromo(ctx context.Context, by int64, in AdminPromoInput) (AdminPromo, error)
	SetPromoEnabled(ctx context.Context, by, id int64, on bool) (AdminPromo, error)
	// DeletePromo removes the code; its activations stay in the history.
	DeletePromo(ctx context.Context, by, id int64) error
	// PromoUses lists a page of the code's activations, newest first.
	PromoUses(ctx context.Context, by, id int64, page int) ([]AdminPromoUse, bool, error)
	// Tariff is one tariff's card.
	Tariff(ctx context.Context, by, id int64) (AdminTariffInfo, error)
	// SetTariffOnSale puts the tariff on sale or takes it off; its subscriptions keep it.
	SetTariffOnSale(ctx context.Context, by, id int64, on bool) (AdminTariffInfo, error)
	// SetTariffPrice sets the price of the tariff's first term: rubles (in kopecks) for RUB,
	// Stars for XTR; 0 takes the price off.
	SetTariffPrice(ctx context.Context, by, id int64, currency string, amount int64) (AdminTariffInfo, error)
	// Tops is a top list of the kind (TopTraffic, TopMoney, TopShare).
	Tops(ctx context.Context, by int64, kind string) ([]AdminTop, error)
	// BroadcastAudience is how many accounts a broadcast would reach now.
	BroadcastAudience(ctx context.Context, by int64) (int, error)
	// Broadcast queues the text for every account with a subscription and returns how many
	// will get it. One at a time: a second one while it is being sent is refused.
	Broadcast(ctx context.Context, by int64, text string) (int, error)
	// NoteGrowth writes the referral programme's new settings to the audit log under the
	// admin's account. The bot calls it before it saves them, so no change goes unrecorded.
	NoteGrowth(ctx context.Context, by int64, enabled bool, inviterDays, friendDays int64) error
}
