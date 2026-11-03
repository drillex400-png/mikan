package nodesync

import (
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"mikan/internal/nodetls"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store"
	"mikan/internal/panel/store/db"
)

// fakeGeo is a geodata source that counts what it is asked and answers as told.
type fakeGeo struct {
	err       error
	ensured   []int64
	forgotten []int64
}

func (g *fakeGeo) Ensure(_ context.Context, _ interface {
	PutGeodata(ctx context.Context, id int64, name string, data []byte) error
}, id int64) error {
	g.ensured = append(g.ensured, id)
	return g.err
}
func (g *fakeGeo) Forget(id int64) { g.forgotten = append(g.forgotten, id) }

// cascadeToB makes node A send its first inbound through a new node B. It returns A's
// syncer and fake node, the inbound, the store and B's id.
func cascadeToB(t *testing.T) (a *Syncer, an *fakeNode, in db.Inbound, st *store.Store, b int64) {
	t.Helper()
	a, an, st, _, _ = setup(t)
	ctx := context.Background()
	q := st.Q
	if err := settings.Set(ctx, settings.New(q), settings.KeyPublicHost, "203.0.113.10"); err != nil {
		t.Fatal(err)
	}
	panel, _ := nodetls.Generate("mikan-panel", x509.ExtKeyUsageClientAuth, time.Now())
	nb, _, err := domain.AddNode(ctx, st, panel, domain.NodeInput{Name: "B", Host: "198.51.100.20", APIPort: 40000}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a.m.attach(t, nb.ID, Target{Node: &fakeNode{}, TLS: fakeTLS})
	if err := domain.CheckExit(ctx, q, a.id, nb.ID); err != nil {
		t.Fatal(err)
	}
	x, err := q.GetNode(ctx, nb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.EnsureRelay(ctx, q, x, time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.RelayUser(ctx, q, nb.ID, a.id); err != nil {
		t.Fatal(err)
	}
	ins, _ := q.ListInbounds(ctx)
	for _, i := range ins {
		if i.NodeID == a.id {
			in = i
			break
		}
	}
	if err := q.SetInboundExit(ctx, db.SetInboundExitParams{ExitNodeID: nullID(nb.ID), Outbound: "direct", ID: in.ID}); err != nil {
		t.Fatal(err)
	}
	return a, an, in, st, nb.ID
}

func ptr(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: true} }

// The mode is in the state only for the inbound that has it on; off is the default.
func TestRUDirectInState(t *testing.T) {
	a, _, in, st, _ := cascadeToB(t)
	ctx := context.Background()
	q := st.Q

	ds, err := a.desired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Exits) != 1 || len(ds.Exits[0].RUDirect) != 0 {
		t.Fatalf("on by itself: %+v", ds.Exits)
	}

	if err := q.SetInboundRUDirect(ctx, ptr(in.ID)); err != nil {
		t.Fatal(err)
	}
	ds, _ = a.desired(ctx)
	if !slices.Equal(ds.Exits[0].RUDirect, []string{in.Name}) {
		t.Fatalf("RUDirect = %v, want [%s]", ds.Exits[0].RUDirect, in.Name)
	}
	if !slices.Contains(ds.Exits[0].Inbounds, in.Name) {
		t.Fatal("the inbound must still go to the exit")
	}

	if err := q.ClearInboundRUDirect(ctx, ptr(in.ID)); err != nil {
		t.Fatal(err)
	}
	ds, _ = a.desired(ctx)
	if len(ds.Exits[0].RUDirect) != 0 {
		t.Fatalf("still on after clearing: %v", ds.Exits[0].RUDirect)
	}
}

// An inbound that has the mode but no exit keeps no rule: there is nothing to split.
func TestRUDirectNeedsExit(t *testing.T) {
	a, _, in, st, _ := cascadeToB(t)
	ctx := context.Background()
	if err := st.Q.SetInboundRUDirect(ctx, ptr(in.ID)); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.SetInboundExit(ctx, db.SetInboundExitParams{Outbound: "direct", ID: in.ID}); err != nil {
		t.Fatal(err)
	}
	ds, _ := a.desired(ctx)
	for _, e := range ds.Exits {
		if len(e.RUDirect) != 0 {
			t.Fatalf("a mode without an exit reached the state: %+v", e)
		}
	}
}

// The databases are asked for only when the state wants the mode; when they cannot be had
// the mode stays (the node keeps the Russian zones on itself) and the inbound keeps its exit.
func TestGeodataGate(t *testing.T) {
	a, _, in, st, _ := cascadeToB(t)
	ctx := context.Background()
	g := &fakeGeo{}
	a.m.SetGeo(g)

	// Off: the source is never touched.
	ds, _ := a.desired(ctx)
	a.geodata(ctx, &ds)
	if len(g.ensured) != 0 {
		t.Fatalf("the source was asked with the mode off: %v", g.ensured)
	}

	if err := st.Q.SetInboundRUDirect(ctx, ptr(in.ID)); err != nil {
		t.Fatal(err)
	}

	// On, databases there: the mode stays.
	ds, _ = a.desired(ctx)
	a.geodata(ctx, &ds)
	if len(g.ensured) != 1 || g.ensured[0] != a.id || len(ds.Exits[0].RUDirect) != 1 {
		t.Fatalf("ensured %v, state %+v", g.ensured, ds.Exits)
	}

	// On, the source fails: the mode stays, the inbound still goes to the exit.
	g.err = errors.New("no database")
	ds, _ = a.desired(ctx)
	a.geodata(ctx, &ds)
	if !slices.Equal(ds.Exits[0].RUDirect, []string{in.Name}) {
		t.Fatalf("the mode was dropped without databases: %v", ds.Exits[0].RUDirect)
	}
	if !slices.Contains(ds.Exits[0].Inbounds, in.Name) {
		t.Fatal("the inbound lost its exit with the mode")
	}

	// No source at all: the same.
	a.m.SetGeo(nil)
	ds, _ = a.desired(ctx)
	a.geodata(ctx, &ds)
	if !slices.Equal(ds.Exits[0].RUDirect, []string{in.Name}) {
		t.Fatal("the mode was dropped with no source")
	}
}

// What reaches the node: the mode, whether the databases are there or not. The node itself
// leaves out the rules it cannot read (see exitRules).
func TestApplyWithoutGeodata(t *testing.T) {
	a, an, in, st, _ := cascadeToB(t)
	ctx := context.Background()
	g := &fakeGeo{err: errors.New("down")}
	a.m.SetGeo(g)
	if err := st.Q.SetInboundRUDirect(ctx, ptr(in.ID)); err != nil {
		t.Fatal(err)
	}

	a.applyState(ctx)
	if len(an.applied) == 0 {
		t.Fatal("nothing applied")
	}
	if first := an.applied[len(an.applied)-1]; len(first.Exits) != 1 || !slices.Equal(first.Exits[0].RUDirect, []string{in.Name}) {
		t.Fatalf("the mode did not reach the node without databases: %+v", first.Exits)
	}

	g.err = nil
	a.applyState(ctx)
	last := an.applied[len(an.applied)-1]
	if len(last.Exits) != 1 || !slices.Equal(last.Exits[0].RUDirect, []string{in.Name}) {
		t.Fatalf("the mode did not reach the node once the databases were there: %+v", last.Exits)
	}
}

// The mode turns itself on when an inbound or a relay is given an exit, and keeping the
// same exit leaves the admin's choice alone.
func TestAutoRUDirect(t *testing.T) {
	_, _, in, st, b := cascadeToB(t)
	ctx := context.Background()
	q := st.Q
	on := func() bool {
		ids, err := q.ListRUDirectInbounds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(ids, ptr(in.ID))
	}

	if err := domain.AutoRUDirect(ctx, q, in.ID, sql.NullInt64{}, sql.NullInt64{}); err != nil || on() {
		t.Fatalf("on without an exit: err=%v", err)
	}
	if err := domain.AutoRUDirect(ctx, q, in.ID, sql.NullInt64{}, ptr(b)); err != nil || !on() {
		t.Fatalf("off after an exit was added: err=%v", err)
	}
	if err := q.ClearInboundRUDirect(ctx, ptr(in.ID)); err != nil {
		t.Fatal(err)
	}
	if err := domain.AutoRUDirect(ctx, q, in.ID, ptr(b), ptr(b)); err != nil || on() {
		t.Fatalf("the same exit overrode the admin: err=%v", err)
	}
	if err := domain.AutoRUDirect(ctx, q, in.ID, ptr(b+1000), ptr(b)); err != nil || !on() {
		t.Fatalf("off after the exit changed: err=%v", err)
	}

	relayOn := func(id int64) bool {
		ids, err := q.ListRUDirectRelays(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(ids, ptr(id))
	}
	// The relay lives on b (cascadeToB made it); a's node is a stand-in for its next hop.
	if err := domain.AutoRelayRUDirect(ctx, q, b, sql.NullInt64{}, ptr(in.NodeID)); err != nil || !relayOn(b) {
		t.Fatalf("relay off after an exit was added: err=%v", err)
	}
}

// Rows of the mode go with the inbound or the relay they belong to, and a row names one.
func TestRUDirectRows(t *testing.T) {
	_, _, in, st, b := cascadeToB(t)
	ctx := context.Background()
	q := st.Q

	// Twice is once.
	for range 2 {
		if err := q.SetInboundRUDirect(ctx, ptr(in.ID)); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.SetRelayRUDirect(ctx, ptr(b)); err != nil {
		t.Fatal(err)
	}
	ins, _ := q.ListRUDirectInbounds(ctx)
	rel, _ := q.ListRUDirectRelays(ctx)
	if len(ins) != 1 || len(rel) != 1 {
		t.Fatalf("inbounds %v relays %v", ins, rel)
	}

	// A row with neither, or both, is refused by the table.
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO cascade_ru_direct DEFAULT VALUES`); err == nil {
		t.Fatal("a row naming nothing was accepted")
	}
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO cascade_ru_direct (inbound_id, relay_node_id) VALUES ($1, $2)`, in.ID, b); err == nil {
		t.Fatal("a row naming both was accepted")
	}

	// Deleting the inbound takes its row, and the node's relay takes its own.
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM inbounds WHERE id = $1`, in.ID); err != nil {
		t.Fatal(err)
	}
	if ins, _ := q.ListRUDirectInbounds(ctx); len(ins) != 0 {
		t.Fatalf("the row outlived its inbound: %v", ins)
	}
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM node_relays WHERE node_id = $1`, b); err != nil {
		t.Fatal(err)
	}
	if rel, _ := q.ListRUDirectRelays(ctx); len(rel) != 0 {
		t.Fatalf("the row outlived its relay: %v", rel)
	}
}
