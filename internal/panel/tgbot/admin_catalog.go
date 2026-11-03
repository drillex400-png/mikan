package tgbot

// What the admin manages of the shop from the bot: promo codes, tariffs, and the clients that
// stand out (the most traffic, the most money, a subscription that looks shared).

// AdminPromo is a promo code as its card shows it.
type AdminPromo struct {
	ID       int64
	Code     string
	Name     string
	Type     string // days, traffic, percent or fixed
	Value    int64  // days, bytes, percent, or the smallest unit of Currency
	Currency string // RUB, XTR or "" (any) for a percent
	Uses     int64
	MaxUses  int64 // 0: no limit
	PerUser  int64
	StartsAt int64 // 0: at once
	EndsAt   int64 // 0: never
	Enabled  bool
	// FirstOnly: on the first purchase only; NewOnly: for new clients only; Tariffs: how many
	// tariffs it is limited to (0: any).
	FirstOnly, NewOnly bool
	Tariffs            int
	CreatedAt          int64
}

// AdminPromoInput is a promo code the admin makes from the bot: one type and value, an
// optional limit of uses and a lifetime in days (0: forever). Code "" lets the panel pick one.
type AdminPromoInput struct {
	Code     string
	Type     string
	Value    int64
	Currency string
	MaxUses  int64
	Days     int64
}

// AdminPromoUse is one activation of a promo code.
type AdminPromoUse struct {
	UserID   int64
	User     string
	TgID     int64
	Status   string
	At       int64
	Days     int64
	Bytes    int64
	Discount int64
	Currency string
}

// AdminPromoStats are the promo codes' totals.
type AdminPromoStats struct {
	Codes, Active, Uses, Days, Bytes, Orders int64
}

// AdminTariffInfo is a tariff as its card shows it.
type AdminTariffInfo struct {
	AdminTariff
	Speed       int64 // Mbit/s; 0: no limit
	Terms       int   // terms it sells beyond the first
	Users       int64 // subscriptions on it
	Description string
}

// The kinds of AdminTop lists.
const (
	TopTraffic = "traffic" // the traffic used in the period
	TopMoney   = "money"   // the applied payments, in rubles and in Stars
	TopShare   = "share"   // subscriptions that look shared
)

// AdminTop is one line of a top list. Value is the kind's figure: bytes, kopecks; Stars is the
// Stars a client paid. For TopShare, IPs are the different addresses its devices came from in
// the last day and Score how strongly it looks shared.
type AdminTop struct {
	AdminUser
	Value, Stars int64
	IPs          int64
	Score        int64
}

// The bounds of what the bot lets the admin set on the shop.
const (
	MaxPromoDays    = 3650
	MaxPromoUses    = 1_000_000
	MaxPromoTraffic = 10_000 // GB
	MaxPromoCode    = 32
	MaxPriceRub     = 1_000_000 // rubles
	MaxPriceStars   = 100_000
)
