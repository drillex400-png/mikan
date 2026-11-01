package app

import (
	"context"
	"crypto/x509"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"mikan/internal/nodetls"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
)

// "Russia direct" over HTTP: off by default; it can be turned on for an inbound that leaves
// through another node and for a relay that sends on, not for one with nothing to split;
// it shows in the state, goes off again, and the admin's journal has each change.
func TestRUDirectOverHTTP(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := domain.Seed(ctx, h.st, h.now); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(ctx, settings.New(h.st.Q), settings.KeyPanelPort, 21355); err != nil {
		t.Fatal(err)
	}
	panel, _ := nodetls.Generate("mikan-panel", x509.ExtKeyUsageClientAuth, time.Now())
	b, _, err := domain.AddNode(ctx, h.st, panel, domain.NodeInput{Name: "B", Host: "198.51.100.20", APIPort: 40000}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := domain.AddNode(ctx, h.st, panel, domain.NodeInput{Name: "C", Host: "198.51.100.30", APIPort: 40000}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if resp, _ := h.login(password, ""); resp.StatusCode != http.StatusOK {
		t.Fatal("login")
	}
	csrf := map[string]string{"X-CSRF-Token": h.csrf}
	api := "/" + adminPath + "/api/v1"
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	ins, _ := h.st.Q.ListInbounds(ctx)
	mine := ins[0]
	ru := api + "/nodes/1/cascade/ru-direct"

	// Off to begin with.
	resp, body := h.do(http.MethodGet, ru, nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"inbounds":[]`) || !strings.Contains(string(body), `"relay":false`) {
		t.Fatalf("initial: %d %s", resp.StatusCode, body)
	}

	// No exit yet: nothing to split.
	resp, body = h.do(http.MethodPut, ru, map[string]any{"inbound": mine.Name, "on": true}, csrf)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), "ru_direct_no_exit") {
		t.Fatalf("no exit: %d %s", resp.StatusCode, body)
	}
	// The relay of a node nobody leaves through, no target, both targets, an unknown inbound.
	if resp, body = h.do(http.MethodPut, ru, map[string]any{"relay": true, "on": true}, csrf); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("relay with no exit: %d %s", resp.StatusCode, body)
	}
	if resp, body = h.do(http.MethodPut, ru, map[string]any{"on": true}, csrf); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), "ru_direct_one_target") {
		t.Fatalf("no target: %d %s", resp.StatusCode, body)
	}
	if resp, body = h.do(http.MethodPut, ru, map[string]any{"inbound": mine.Name, "relay": true, "on": true}, csrf); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("two targets: %d %s", resp.StatusCode, body)
	}
	if resp, body = h.do(http.MethodPut, ru, map[string]any{"inbound": "nope", "on": true}, csrf); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown inbound: %d %s", resp.StatusCode, body)
	}
	// Without the CSRF token a write is refused.
	if resp, _ = h.do(http.MethodPut, ru, map[string]any{"inbound": mine.Name, "on": true}, nil); resp.StatusCode == http.StatusOK {
		t.Fatal("a write without the CSRF token went through")
	}

	// Through B: now it can be turned on, and it shows.
	if resp, body = h.do(http.MethodPatch, api+"/inbounds/"+id(mine.ID), map[string]any{"outbound": "node", "exit_node_id": b.ID}, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("via B: %d %s", resp.StatusCode, body)
	}
	resp, body = h.do(http.MethodPut, ru, map[string]any{"inbound": mine.Name, "on": true}, csrf)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"inbounds":["`+mine.Name+`"]`) {
		t.Fatalf("on: %d %s", resp.StatusCode, body)
	}
	// Twice is once.
	if resp, body = h.do(http.MethodPut, ru, map[string]any{"inbound": mine.Name, "on": true}, csrf); resp.StatusCode != http.StatusOK || strings.Count(string(body), mine.Name) != 1 {
		t.Fatalf("on twice: %d %s", resp.StatusCode, body)
	}

	// B's relay goes on to C: its mode can be turned on too, and is its own.
	if resp, body = h.do(http.MethodPatch, api+"/nodes/"+id(b.ID)+"/cascade", map[string]any{"outbound": "node", "exit_node_id": c.ID}, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("B to C: %d %s", resp.StatusCode, body)
	}
	ruB := api + "/nodes/" + id(b.ID) + "/cascade/ru-direct"
	resp, body = h.do(http.MethodPut, ruB, map[string]any{"relay": true, "on": true}, csrf)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"relay":true`) {
		t.Fatalf("relay on: %d %s", resp.StatusCode, body)
	}
	if _, body = h.do(http.MethodGet, ru, nil, nil); !strings.Contains(string(body), `"relay":false`) {
		t.Fatalf("A's relay took B's switch: %s", body)
	}

	// Off again.
	resp, body = h.do(http.MethodPut, ru, map[string]any{"inbound": mine.Name, "on": false}, csrf)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"inbounds":[]`) {
		t.Fatalf("off: %d %s", resp.StatusCode, body)
	}

	// The journal has each change: on, on again, relay on, off.
	var n int
	if err := h.st.DB.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE action = 'node.cascade_ru_direct'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("%d audit rows for the mode, want 4", n)
	}
}
