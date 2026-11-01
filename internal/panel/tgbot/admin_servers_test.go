package tgbot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode"

	"mikan/internal/panel/model"
)

// Every screen of the servers draws, escapes the names, and its buttons fit Telegram's limits
// and lead to codes the bot answers.
func TestServerScreensDraw(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	for _, d := range []string{"an", "aS:1", "aS:1:c", "aS:1:s", "aS:1:e", "aS:1:u", "aS:1:r", "aS:1:d", "aS:1:d:2", "aS:1:n", "aS:1:p", "aS:1:m",
		"aS:1:z:e0", "aS:1:z:e1", "aS:1:z:u", "aS:1:z:r"} {
		text, kb := b.adminScreen(ctx, boss, d, "")
		if text == "" {
			t.Errorf("%s: no text", d)
		}
		if strings.Contains(text, "NL-<1>") {
			t.Errorf("%s: the name is not escaped:\n%s", d, text)
		}
		if kb == nil {
			continue
		}
		for _, row := range kb.InlineKeyboard {
			if len(row) == 0 || len(row) > 4 {
				t.Errorf("%s: a row of %d buttons", d, len(row))
			}
			for _, bt := range row {
				if bt.Text == "" || len(bt.CallbackData) == 0 || len(bt.CallbackData) > 64 {
					t.Errorf("%s: bad button %+v", d, bt)
				}
				cmd, _, _ := strings.Cut(bt.CallbackData, ":")
				if _, ok := b.pressKnown(cmd); !ok {
					t.Errorf("%s: button %q leads to a code the bot does not answer", d, bt.CallbackData)
				}
			}
		}
	}
}

// The list offers each server as a button to its card.
func TestServerListLeadsToCards(t *testing.T) {
	b := screenBot(&fakeAdmin{user: testUser()})
	_, kb := b.adminScreen(context.Background(), boss, "an", "")
	var got []string
	for _, row := range kb.InlineKeyboard {
		for _, bt := range row {
			got = append(got, bt.CallbackData)
		}
	}
	for _, want := range []string{"aS:1", "aS:2"} {
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("no button %q in %v", want, got)
		}
	}
}

// Nothing is changed by asking: only the confirmed code acts.
func TestServerAsksDoNotAct(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	for _, d := range []string{"aS:1:e", "aS:1:u", "aS:1:r", "aS:1:d", "aS:1:d:2", "aS:1:n", "aS:1:p", "aS:1:m"} {
		b.adminScreen(context.Background(), boss, d, "")
	}
	if len(adm.calls) != 0 {
		t.Fatalf("asking acted: %v", adm.calls)
	}
}

// Every "yes" leads to the change it asked about; a removal takes two asks.
func TestServerYesButtons(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	b := screenBot(adm)
	ctx := context.Background()
	for ask, want := range map[string]string{
		"aS:1:e": "aS:1:z:e0", "aS:1:u": "aS:1:z:u", "aS:1:r": "aS:1:z:r", "aS:1:d": "aS:1:d:2", "aS:1:d:2": "aS:1:z:d",
	} {
		_, kb := b.adminScreen(ctx, boss, ask, "")
		row := kb.InlineKeyboard[0]
		if row[0].CallbackData != want || row[1].CallbackData != "aS:1" {
			t.Errorf("%s: yes=%q no=%q, want %q / aS:1", ask, row[0].CallbackData, row[1].CallbackData, want)
		}
	}
	// A server that is off is offered to be turned on.
	adm.node = AdminNodeInfo{AdminNode: AdminNode{ID: 1, Name: "x"}}
	_, kb := b.adminScreen(ctx, boss, "aS:1:e", "")
	if kb.InlineKeyboard[0][0].CallbackData != "aS:1:z:e1" {
		t.Errorf("an off server is asked to be turned %q", kb.InlineKeyboard[0][0].CallbackData)
	}
}

// The confirmed codes reach the panel.
func TestServerConfirmedActions(t *testing.T) {
	for code, want := range map[string]string{
		"aS:1:z:e0": "node_off", "aS:1:z:e1": "node_on", "aS:1:z:u": "node_update", "aS:1:z:r": "node_restart", "aS:1:z:d": "node_delete",
		"aS:1:c": "node_check", "aS:1:s": "node_speed",
	} {
		adm := &fakeAdmin{user: testUser()}
		b := screenBot(adm)
		b.adminScreen(context.Background(), boss, code, "")
		if len(adm.calls) != 1 || adm.calls[0] != want {
			t.Errorf("%s: calls %v, want [%s]", code, adm.calls, want)
		}
	}
}

// The card of a server that is gone, or an unknown one, is "not found", not a crash.
func TestServerNotFound(t *testing.T) {
	b := screenBot(&fakeAdmin{user: testUser()})
	for _, d := range []string{"aS:99", "aS:0", "aS:x", "aS:99:e"} {
		text, _ := b.adminScreen(context.Background(), boss, d, "")
		if text == "" {
			t.Errorf("%s: empty", d)
		}
	}
}

// A server's own refusal is said in its words, not as a bare failure.
func TestServerRefusals(t *testing.T) {
	b := screenBot(nil)
	sw := srvWordsFor("ru")
	for err, want := range map[error]string{
		model.ErrNodeLocal: sw.eNodeLocal, model.ErrNodeOffline: sw.eNodeOffline, model.ErrNodeNotBehind: sw.eNotBehind,
		model.ErrNodeUpdating: sw.eUpdating, model.ErrNodeCannot: sw.eCannot, model.ErrNodeInUse: sw.eInUse, model.ErrNodeFailed: sw.eFailed,
	} {
		adm := &fakeAdmin{user: testUser(), nodeErr: err}
		b = screenBot(adm)
		text, _ := b.adminScreen(context.Background(), boss, "aS:1:z:u", "")
		if !strings.Contains(text, want[:12]) {
			t.Errorf("%v: the screen says\n%s\nwant %q", err, text, want)
		}
	}
	if _, ok := srvFail(sw, errors.New("other")); ok {
		t.Error("a foreign error is told as a server's")
	}
}

// The card holds no key, certificate or API address, only what the admin reads.
func TestServerCardHasNoSecrets(t *testing.T) {
	adm := &fakeAdmin{user: testUser()}
	adm.node = AdminNodeInfo{AdminNode: AdminNode{ID: 1, Name: "NL", Host: "nl.example", Enabled: true, Online: true}, Status: "ok"}
	b := screenBot(adm)
	text, _ := b.adminScreen(context.Background(), boss, "aS:1", "")
	// Whole words: "Подключения" holds "ключ" and is not a key.
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		switch w {
		case "key", "ключ", "ключа", "cert", "certificate", "сертификат", "token", "токен", "secret", "секрет", "password", "пароль":
			t.Errorf("the card mentions %q:\n%s", w, text)
		}
	}
}

// Text for a name, a public name and the channel: bad input is turned back with a notice, good
// input is passed on as the field and value the panel is told.
func TestServerTextInput(t *testing.T) {
	sw := srvWordsFor("ru")
	for _, c := range []struct{ kind, text, field, value string }{
		{"nname", "Берлин", NodeFieldName, "Берлин"},
		{"npublic", "-", NodeFieldPublic, ""},
		{"npublic", "Public", NodeFieldPublic, "Public"},
		{"nchannel", "500", NodeFieldChannel, "500"},
		{"nchannel", "100000", NodeFieldChannel, "100000"},
	} {
		f, v, notice, ok := srvInput(adminState{kind: c.kind, user: 1}, c.text, sw)
		if !ok || notice != "" || f != c.field || v != c.value {
			t.Errorf("%s %q: got %q=%q notice %q ok %v, want %q=%q", c.kind, c.text, f, v, notice, ok, c.field, c.value)
		}
	}
	for _, c := range []struct{ kind, text string }{
		{"nchannel", "abc"}, {"nchannel", "0"}, {"nchannel", "-5"}, {"nchannel", "100001"}, {"nname", ""},
		{"nname", strings.Repeat("я", MaxNodeText+1)}, {"npublic", strings.Repeat("a", MaxNodeText+1)},
	} {
		f, _, notice, ok := srvInput(adminState{kind: c.kind, user: 1}, c.text, sw)
		if !ok || notice == "" || f != "" {
			t.Errorf("%s %q: should be turned back (field %q notice %q ok %v)", c.kind, c.text, f, notice, ok)
		}
	}
	if _, _, _, ok := srvInput(adminState{kind: "note"}, "x", sw); ok {
		t.Error("a note is taken as a server's text")
	}
}
