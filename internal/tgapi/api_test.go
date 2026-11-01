package tgapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A message Telegram cannot parse goes again as plain text instead of failing; any other
// refusal stays an error.
func TestSendFallsBackToPlainText(t *testing.T) {
	var calls []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, body)
		switch {
		case body["parse_mode"] == "HTML":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 400, "description": "Bad Request: can't parse entities: Unsupported start tag \"x\" at byte offset 0"})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 5, "chat": map[string]any{"id": 1, "type": "private"}}})
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "1:x", nil)
	m, err := c.Send(context.Background(), 1, "<x>a &lt; b</x>", nil, false)
	if err != nil || m.MessageID != 5 || len(calls) != 2 || calls[1]["text"] != "a < b" || calls[1]["parse_mode"] != nil {
		t.Fatalf("send: %+v %v %v", m, err, calls)
	}
	calls = nil
	if err := c.Edit(context.Background(), 1, 5, "<x>c</x>", nil); err != nil || len(calls) != 2 || calls[1]["text"] != "c" {
		t.Fatalf("edit: %v %v", err, calls)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 403, "description": "Forbidden: bot was blocked by the user"})
	}))
	defer other.Close()
	if _, err := NewClient(other.URL, "1:x", nil).Send(context.Background(), 1, "a", nil, false); err == nil {
		t.Fatal("a blocked bot is not a parse error")
	}
}

// PlainText is what is left of the HTML when Telegram refuses it: the text, unescaped.
func TestPlainText(t *testing.T) {
	if got := PlainText(`<b>a &lt; b</b> <a href="https://e.com">c &amp; d</a> &#34;e&#34;`); got != `a < b c & d "e"` {
		t.Fatalf("plain %q", got)
	}
}

// The admin's command menu is set for that chat alone (scope "chat"); the bot-wide list keeps
// /start only, so nobody else sees /admin.
func TestSetChatCommandsIsScopedToOneChat(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "1:x", nil)
	if err := c.SetChatCommands(context.Background(), 77, "Menu", "Admin"); err != nil {
		t.Fatal(err)
	}
	sc, _ := got["scope"].(map[string]any)
	if sc["type"] != "chat" || sc["chat_id"] != float64(77) {
		t.Fatalf("the scope is not that one chat: %v", got["scope"])
	}
	cmds, _ := got["commands"].([]any)
	if len(cmds) != 2 {
		t.Fatalf("commands: %v", got["commands"])
	}
	if err := c.SetCommands(context.Background(), "Menu"); err != nil {
		t.Fatal(err)
	}
	if _, scoped := got["scope"]; scoped {
		t.Fatalf("the bot-wide list got a scope: %v", got)
	}
	if cmds, _ := got["commands"].([]any); len(cmds) != 1 {
		t.Fatalf("the bot-wide list must stay /start only: %v", got["commands"])
	}
}
