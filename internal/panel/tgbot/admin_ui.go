package tgbot

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
)

// How the admin's screens look. Everything here is a pure function of its arguments, so the
// look is the same on every screen and a test can pin it. The texts it builds are HTML for
// Telegram: every piece of data goes through html.EscapeString, and the markup is only what
// this file writes.

const (
	barCells = 10
	rule     = "━━━━━━━━━━━━━━━━"
)

// bar draws used/limit as a row of cells with the share beside it: ▰▰▰▱▱▱▱▱▱▱ 32%. A limit of
// 0 is no limit, and there is nothing to fill.
func bar(used, limit int64) string {
	if limit <= 0 {
		return strings.Repeat("▱", barCells) + " ∞"
	}
	if used < 0 {
		used = 0
	}
	pct := used * 100 / limit
	if pct > 100 {
		pct = 100
	}
	// A client who has used anything shows at least one cell, and one who has not shows none.
	filled := int(pct * barCells / 100)
	if used > 0 && filled == 0 {
		filled = 1
	}
	return strings.Repeat("▰", filled) + strings.Repeat("▱", barCells-filled) + fmt.Sprintf(" %d%%", pct)
}

// left says how long is left until at, or how long ago it ended, in the words of aw.
func left(aw *admWords, at, now time.Time) string {
	d := at.Sub(now)
	days := int(d.Hours() / 24)
	switch {
	case d < 0 && -d < 24*time.Hour:
		return aw.endedToday
	case d < 0:
		return fmt.Sprintf(aw.endedAgo, -days)
	case d < 24*time.Hour:
		return aw.endsToday
	}
	return fmt.Sprintf(aw.endsIn, days)
}

// stateLabel is a subscription's state as a coloured mark and a word in the bot's language.
func stateLabel(aw *admWords, s string) string {
	switch s {
	case "active":
		return "🟢 " + aw.stActive
	case "expiring":
		return "🟡 " + aw.stExpiring
	case "expired":
		return "🔴 " + aw.stExpired
	case "limited":
		return "🟠 " + aw.stLimited
	case "disabled":
		return "⛔ " + aw.stDisabled
	}
	return s
}

// stateDot is only the coloured mark, for a row of a list.
func stateDot(s string) string {
	switch s {
	case "active":
		return "🟢"
	case "expiring":
		return "🟡"
	case "expired":
		return "🔴"
	case "limited":
		return "🟠"
	case "disabled":
		return "⛔"
	}
	return "⚪"
}

// payLabel is a payment's status as a mark and a word.
func payLabel(aw *admWords, s string) string {
	switch s {
	case "applied", "paid":
		return "✅ " + aw.pPaid
	case "pending":
		return "⏳ " + aw.pPending
	case "failed":
		return "❌ " + aw.pFailed
	case "expired":
		return "⌛ " + aw.pExpired
	case "refunded":
		return "↩️ " + aw.pRefunded
	}
	return s
}

// payDot is the mark alone, for a row.
func payDot(s string) string {
	switch s {
	case "applied", "paid":
		return "✅"
	case "pending":
		return "⏳"
	case "failed":
		return "❌"
	case "expired":
		return "⌛"
	case "refunded":
		return "↩️"
	}
	return "▫️"
}

// money is an amount in minor units with its currency: 150.00 ₽. Stars have no minor unit.
func money(amount int64, cur string) string {
	switch strings.ToUpper(cur) {
	case "XTR", "STARS":
		return fmt.Sprintf("%d ⭐", amount)
	case "RUB":
		return fmt.Sprintf("%.2f ₽", float64(amount)/100)
	}
	return fmt.Sprintf("%.2f %s", float64(amount)/100, cur)
}

// head is a screen's title: bold, with the rule under it.
func head(icon, title string) string {
	return "<b>" + icon + " " + html.EscapeString(plain(title)) + "</b>\n" + rule
}

// plain is a button's word without the mark it starts with: "👥 Клиенты" is "Клиенты". A
// title carries its own mark, so the button's would show twice.
func plain(s string) string {
	i := strings.IndexByte(s, ' ')
	if i <= 0 {
		return s
	}
	for _, r := range s[:i] {
		if r < 0x2000 { // a letter or a digit: the first word is a word, not a mark
			return s
		}
	}
	return strings.TrimSpace(s[i+1:])
}

// kv is one line of a card: a label and its value, both escaped, the label dimmed by the
// colon alone (Telegram has no colours; bold is kept for titles).
func kv(label, value string) string {
	return html.EscapeString(label) + ": <b>" + html.EscapeString(value) + "</b>"
}

// pill is a number with its caption for the overview, aligned by a fixed-width number.
func pill(icon, label string, n int64) string {
	return fmt.Sprintf("%s %s: <b>%d</b>", icon, html.EscapeString(label), n)
}

// userFilterName is the words of a clients' filter.
func userFilterName(aw *admWords, f string) string {
	switch f {
	case "s":
		return aw.fSoon
	case "e":
		return aw.fEnded
	case "t":
		return aw.fLow
	case "n":
		return aw.fNoTG
	case "b":
		return aw.fBlocked
	}
	return aw.fAll
}

// overviewExtra is the money and the day's figures under the overview. An older panel sends
// none of them, and then there is nothing to add.
func overviewExtra(aw *admWords, o AdminOverview) string {
	var out []string
	if o.NewToday > 0 {
		out = append(out, block("", Row{"🆕", aw.ovNew, strconv.FormatInt(o.NewToday, 10)}))
	}
	if len(o.Revenue) > 0 {
		var rows []Row
		for _, m := range o.Revenue {
			rows = append(rows,
				Row{"▫️", aw.ovToday2, money(m.Today, m.Currency)},
				Row{"▫️", aw.ovWeek, money(m.Week, m.Currency)},
				Row{"▫️", aw.ovMonth, money(m.Month, m.Currency)})
		}
		out = append(out, block("💰 "+aw.ovRevenue, rows...))
	}
	return join(out...)
}
