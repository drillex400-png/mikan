package tgbot

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/events"
)

func ev(t string, tg, user int64, data any) events.Event {
	b, _ := json.Marshal(data)
	return events.Event{Type: t, TgID: tg, UserID: user, Data: b}
}

func TestAdminNotices(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	pay := events.Payment{PaymentID: 7, Kind: "renew", Provider: "stars", Amount: 250, Currency: "XTR", TariffName: "Месяц", TermDays: 30}

	got := adminNotice(&admRU, ev(events.PaymentPaid, 42, 5, pay), "Анна", now)
	for _, want := range []string{"Новая оплата", "250 ⭐", "продление", "Месяц", "Анна", "stars"} {
		if !strings.Contains(got, want) {
			t.Errorf("paid notice lacks %q:\n%s", want, got)
		}
	}
	if got := adminNotice(&admRU, ev(events.PaymentRefunded, 42, 0, pay), "", now); !strings.Contains(got, "Возврат") || !strings.Contains(got, "42") {
		t.Errorf("refund notice (no name: the Telegram id stands in):\n%s", got)
	}
	if got := adminNotice(&admEN, ev(events.UserCreated, 0, 9, events.User{Source: "bot"}), "Bob", now); !strings.Contains(got, "New client") || !strings.Contains(got, "from the bot") {
		t.Errorf("new client notice:\n%s", got)
	}
	down := adminNotice(&admRU, ev(events.NodeDown, 0, 0, events.Node{NodeID: 1, Name: "NL-1", Code: "timeout", Since: now.Unix()}), "", now)
	if !strings.Contains(down, "недоступен") || !strings.Contains(down, "NL-1") {
		t.Errorf("node down:\n%s", down)
	}
	up := adminNotice(&admRU, ev(events.NodeUp, 0, 0, events.Node{NodeID: 1, Name: "NL-1", DownAt: now.Add(-90 * time.Minute).Unix()}), "", now)
	if !strings.Contains(up, "снова работает") || !strings.Contains(up, "1 ч.") {
		t.Errorf("node up:\n%s", up)
	}
}

// Names and labels come from the outside: markup in them must not reach the admin as markup.
func TestAdminNoticeEscapes(t *testing.T) {
	now := time.Now()
	pay := events.Payment{Kind: "new", Provider: "<b>x</b>", Amount: 1, Currency: "RUB", TariffName: "<i>t</i>"}
	got := adminNotice(&admRU, ev(events.PaymentPaid, 1, 1, pay), "<script>", now)
	for _, bad := range []string{"<script>", "<i>t</i>", "<b>x</b>"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q is not escaped:\n%s", bad, got)
		}
	}
	n := adminNotice(&admRU, ev(events.NodeDown, 0, 0, events.Node{Name: "<u>n</u>"}), "", now)
	if strings.Contains(n, "<u>") {
		t.Errorf("node name not escaped:\n%s", n)
	}
}

// An event with nothing to tell, a broken body, or a type the admins are not told of gives no
// notice (and so sends nothing).
func TestAdminNoticeQuiet(t *testing.T) {
	now := time.Now()
	for _, e := range []events.Event{
		{Type: events.PaymentPaid, Data: json.RawMessage(`not json`)},
		{Type: events.NodeDown, Data: json.RawMessage(`{"node_id":1}`)}, // no name
		{Type: events.Traffic90, Data: json.RawMessage(`{}`)},
		{Type: "something.new", Data: json.RawMessage(`{}`)},
	} {
		if got := adminNotice(&admRU, e, "", now); got != "" {
			t.Errorf("%s gave a notice: %s", e.Type, got)
		}
	}
}

// The events the admins are told of are asked of the panel, or they would never arrive.
func TestAdminEventsAreSubscribed(t *testing.T) {
	have := map[string]bool{}
	for _, x := range EventTypes {
		have[x] = true
	}
	for _, x := range AdminEventTypes {
		if !have[x] {
			t.Errorf("%s is told to the admins but not in EventTypes", x)
		}
		if !isAdminEvent(x) {
			t.Errorf("%s is listed but isAdminEvent denies it", x)
		}
	}
	if isAdminEvent(events.Traffic90) {
		t.Error("a subscriber's traffic notice must not go to the admins")
	}
}

// Both languages have every word: an empty one would print as a hole in a screen.
func TestAdminWordsComplete(t *testing.T) {
	for name, aw := range map[string]*admWords{"ru": &admRU, "en": &admEN} {
		v := reflectStrings(aw)
		for field, s := range v {
			if s == "" {
				t.Errorf("%s: %s is empty", name, field)
			}
		}
	}
}
