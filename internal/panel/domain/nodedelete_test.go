package domain

import (
	"context"
	"errors"
	"testing"
	"time"

	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
)

// A remote node goes with the rules of the admin panel: the row and its state are gone and the
// bot's route lets go of it; the bot's own way to Telegram, a tariff left with no servers, the
// panel's own node and a node that is not there stop it, with nothing removed.
func TestDeleteNode(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Unix(1_800_000_000, 0)
	node := func(name string) db.Node {
		t.Helper()
		n, err := st.Q.CreateNode(ctx, db.CreateNodeParams{Name: name, Address: name + ":1", PublicHost: "203.0.113.1", CreatedAt: 1, UpdatedAt: 1})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	exists := func(id int64) bool { _, err := st.Q.GetNode(ctx, id); return err == nil }

	var unrouted []int64
	rel := NodeRelease{
		InUse:   func(context.Context, *db.Queries, int64) (bool, error) { return false, nil },
		Unroute: func(_ context.Context, _ *db.Queries, id int64) error { unrouted = append(unrouted, id); return nil },
	}

	// The panel's own node is kept.
	nodes, _ := st.Q.ListNodes(ctx)
	for _, n := range nodes {
		if n.Address == "" {
			if _, err := DeleteNode(ctx, st, n.ID, now, rel); !errors.Is(err, ErrNodeLocal) || !exists(n.ID) {
				t.Fatalf("the panel's own node: %v", err)
			}
		}
	}
	if _, err := DeleteNode(ctx, st, 987654, now, rel); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("a node that is not there: %v", err)
	}

	// The bot goes to Telegram through the node: it stays, and the route is not touched.
	via := node("via")
	inUse := rel
	inUse.InUse = func(_ context.Context, _ *db.Queries, id int64) (bool, error) { return id == via.ID, nil }
	if _, err := DeleteNode(ctx, st, via.ID, now, inUse); !errors.Is(err, ErrNodeInUse) || !exists(via.ID) || len(unrouted) != 0 {
		t.Fatalf("a node the bot needs: %v exists=%v unrouted=%v", err, exists(via.ID), unrouted)
	}

	// A tariff of chosen servers whose only server this is would be left with none.
	only, spare := node("only"), node("spare")
	tariff, err := st.Q.CreateTariff(ctx, db.CreateTariffParams{Name: "Solo tariff", DurationDays: 30, ResetStrategy: "none", PriceLabel: "x", CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.SetTariffChosenServers(ctx, db.SetTariffChosenServersParams{ID: tariff.ID, ChosenServers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.AddTariffNode(ctx, db.AddTariffNodeParams{TariffID: tariff.ID, NodeID: only.ID}); err != nil {
		t.Fatal(err)
	}
	var left *TariffServersError
	if _, err := DeleteNode(ctx, st, only.ID, now, rel); !errors.As(err, &left) || len(left.Names) != 1 || left.Names[0] != "Solo tariff" || !exists(only.ID) {
		t.Fatalf("a tariff left with no servers: %v exists=%v", err, exists(only.ID))
	}
	// With another server on the tariff the first one can go.
	if err := st.Q.AddTariffNode(ctx, db.AddTariffNodeParams{TariffID: tariff.ID, NodeID: spare.ID}); err != nil {
		t.Fatal(err)
	}
	gone, err := DeleteNode(ctx, st, only.ID, now, rel)
	if err != nil || exists(only.ID) || gone.Node.ID != only.ID || gone.Node.Name != "only" {
		t.Fatalf("removal: %+v %v exists=%v", gone, err, exists(only.ID))
	}
	if len(unrouted) != 1 || unrouted[0] != only.ID {
		t.Errorf("the route was not let go: %v", unrouted)
	}
}
