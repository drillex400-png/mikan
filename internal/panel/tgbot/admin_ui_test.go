package tgbot

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestBar(t *testing.T) {
	for _, c := range []struct {
		used, limit int64
		want        string
	}{
		{0, 0, "▱▱▱▱▱▱▱▱▱▱ ∞"},
		{5 << 30, 0, "▱▱▱▱▱▱▱▱▱▱ ∞"},
		{0, 100, "▱▱▱▱▱▱▱▱▱▱ 0%"},
		{1, 100, "▰▱▱▱▱▱▱▱▱▱ 1%"}, // used anything: at least one cell
		{50, 100, "▰▰▰▰▰▱▱▱▱▱ 50%"},
		{100, 100, "▰▰▰▰▰▰▰▰▰▰ 100%"},
		{250, 100, "▰▰▰▰▰▰▰▰▰▰ 100%"}, // over the limit stays full
		{-5, 100, "▱▱▱▱▱▱▱▱▱▱ 0%"},
	} {
		if got := bar(c.used, c.limit); got != c.want {
			t.Errorf("bar(%d, %d) = %q, want %q", c.used, c.limit, got, c.want)
		}
	}
	// Always ten cells, so the cards line up.
	for u := int64(0); u <= 120; u += 7 {
		g := bar(u, 100)
		cells := strings.Count(g, "▰") + strings.Count(g, "▱")
		if cells != barCells {
			t.Errorf("bar(%d, 100) has %d cells", u, cells)
		}
	}
}

func TestLeft(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{now.Add(12 * 24 * time.Hour), "ещё 12 дн."},
		{now.Add(3 * time.Hour), "заканчивается сегодня"},
		{now.Add(-3 * time.Hour), "закончилась сегодня"},
		{now.Add(-5 * 24 * time.Hour), "закончилась 5 дн. назад"},
	} {
		if got := left(&admRU, c.at, now); got != c.want {
			t.Errorf("left(%v) = %q, want %q", c.at.Sub(now), got, c.want)
		}
	}
	if got := left(&admEN, now.Add(2*24*time.Hour+time.Hour), now); got != "2 d left" {
		t.Errorf("english: %q", got)
	}
}

func TestMoney(t *testing.T) {
	for _, c := range []struct {
		n    int64
		cur  string
		want string
	}{
		{15000, "RUB", "150.00 ₽"},
		{250, "XTR", "250 ⭐"},
		{1999, "USD", "19.99 USD"},
	} {
		if got := money(c.n, c.cur); got != c.want {
			t.Errorf("money(%d, %s) = %q, want %q", c.n, c.cur, got, c.want)
		}
	}
}

// Every state and payment status a list can show has a mark, and an unknown one is shown as it
// is rather than hidden.
func TestLabels(t *testing.T) {
	for _, s := range []string{"active", "expiring", "expired", "limited", "disabled"} {
		if l := stateLabel(&admRU, s); l == s || !strings.HasPrefix(l, stateDot(s)) {
			t.Errorf("state %s: %q", s, l)
		}
	}
	for _, s := range []string{"applied", "paid", "pending", "failed", "expired", "refunded"} {
		if l := payLabel(&admRU, s); l == s || !strings.HasPrefix(l, payDot(s)) {
			t.Errorf("payment %s: %q", s, l)
		}
	}
	if stateLabel(&admRU, "weird") != "weird" || payLabel(&admRU, "weird") != "weird" {
		t.Error("an unknown status must be shown as it is")
	}
}

// What the admin's data holds is escaped: a name with markup cannot inject any into a screen.
func TestKVEscapes(t *testing.T) {
	got := kv("a<b", "<script>&")
	if strings.Contains(got, "<script>") || strings.Contains(got, "a<b") {
		t.Errorf("not escaped: %s", got)
	}
	if !strings.Contains(head("👤", "<i>x</i>"), "&lt;i&gt;") {
		t.Error("head does not escape")
	}
}

// Telegram refuses callback data over 64 bytes, and a button with none: every code the screens
// build, with the largest ids, must fit.
func TestCallbackDataFits(t *testing.T) {
	const big = "9223372036854775807"
	for _, d := range []string{
		"ak:" + big, "ae:" + big + ":365", "ac:" + big + ":1", "ag:" + big + ":100000", "av:" + big + ":1000",
		"ar:" + big, "ai:" + big, "ax:" + big + ":2", "aN:" + big + ":s", "at:" + big, "aT:" + big + ":" + big,
		"az:t:" + big + ":" + big, "az:g:" + big + ":100000", "aP:" + big + ":p:50", "ap:r:50", "au:50",
	} {
		if n := len(d); n > 64 {
			t.Errorf("%q is %d bytes", d, n)
		}
	}
}

// The filter letters map to real payment statuses and back to a safe default.
func TestPayFilter(t *testing.T) {
	for l, want := range map[string]string{"": "", "p": "applied", "w": "pending", "f": "failed", "r": "refunded", "zz": ""} {
		if got := payFilterStatus(l); got != want {
			t.Errorf("filter %q = %q, want %q", l, got, want)
		}
	}
}

// A button's text stays short and never cuts a letter in half.
func TestTruncateRunes(t *testing.T) {
	if got := truncate("Привет, мир! Это длинное имя клиента", 12); utf8.RuneCountInString(got) != 12 || !utf8.ValidString(got) {
		t.Errorf("truncate: %q", got)
	}
}

// reflectStrings lists the string fields of a struct by name.
func reflectStrings(v any) map[string]string {
	out := map[string]string{}
	rv := reflect.ValueOf(v).Elem()
	for i := 0; i < rv.NumField(); i++ {
		if rv.Field(i).Kind() == reflect.String {
			out[rv.Type().Field(i).Name] = rv.Field(i).String()
		}
	}
	return out
}
