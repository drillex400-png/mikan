package tgapi

import (
	"errors"
	"testing"
)

// The colours come off a keyboard and nothing else changes in it; the original keeps its own.
func TestKeyboardUnpainted(t *testing.T) {
	k := &Keyboard{InlineKeyboard: [][]Button{{{Text: "a", CallbackData: "x", Style: StyleDanger}, {Text: "b", URL: "u"}}}}
	u := k.Unpainted()
	if u.InlineKeyboard[0][0].Style != "" || u.InlineKeyboard[0][0].CallbackData != "x" || u.InlineKeyboard[0][1].URL != "u" {
		t.Fatalf("unpainted: %+v", u)
	}
	if k.InlineKeyboard[0][0].Style != StyleDanger {
		t.Fatal("the original lost its colour")
	}
}

// Only a refusal of the keyboard itself is retried without colours.
func TestBadButtons(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{&APIError{Code: 400, Description: "Bad Request: field \"style\" is not allowed"}, true},
		{&APIError{Code: 400, Description: "Bad Request: BUTTON_DATA_INVALID"}, true},
		{&APIError{Code: 400, Description: "Bad Request: can't parse entities"}, false},
		{&APIError{Code: 403, Description: "Forbidden: bot was blocked button"}, false},
		{errors.New("network"), false},
		{nil, false},
	} {
		if got := badButtons(c.err); got != c.want {
			t.Errorf("%v: %v", c.err, got)
		}
	}
}
