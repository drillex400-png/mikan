package domain

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"mikan/internal/panel/store"
	"mikan/internal/panel/store/db"
)

var (
	// ErrNodeLocal: the panel's own node is not removed.
	ErrNodeLocal = errors.New("local_node")
	// ErrNodeInUse: the bot goes to Telegram through this node and would lose its way.
	ErrNodeInUse = errors.New("node_in_use")
)

// TariffServersError: removing the node would leave these tariffs of chosen servers with none.
type TariffServersError struct{ Names []string }

func (e *TariffServersError) Error() string {
	return "tariff_servers: " + strings.Join(e.Names, ", ")
}

// NodeGone is what a removal did, for the caller to finish outside the database.
type NodeGone struct {
	Node  db.Node
	Moved []MovedExit
}

// NodeRelease is what only the callers can do about a node that is going: the bot's route
// through it (the tgate package, which depends on this one), and the next node for what left
// through it.
type NodeRelease struct {
	// InUse says whether the node is the one the bot goes to Telegram through.
	InUse func(ctx context.Context, q *db.Queries, id int64) (bool, error)
	// Unroute lets the bot's route go of the node.
	Unroute func(ctx context.Context, q *db.Queries, id int64) error
	// Host tells a node's ports for a new exit.
	Host HostLookup
}

// DeleteNode removes a remote node in one transaction the way the admin panel does: what went
// out through it goes through another node or straight out, a node the bot needs or a tariff
// that would be left with no servers stops it, the bot's route and the node's state go.
// Errors: ErrUnknownNode, ErrNodeLocal, ErrNodeInUse, *TariffServersError. The caller then
// retires the node, wakes the syncer and clears the node's files.
func DeleteNode(ctx context.Context, st *store.Store, id int64, now time.Time, rel NodeRelease) (NodeGone, error) {
	var gone NodeGone
	n, err := st.Q.GetNode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return gone, ErrUnknownNode
	}
	if err != nil {
		return gone, err
	}
	if n.Address == "" {
		return gone, ErrNodeLocal
	}
	gone.Node = n
	err = st.Tx(ctx, func(q *db.Queries) error {
		var err error
		// What went out through the node goes through another one, or straight out: the
		// cascade does not keep the node from going.
		if gone.Moved, err = MoveExits(ctx, q, n.ID, now, rel.Host); err != nil {
			return err
		}
		if rel.InUse != nil {
			use, err := rel.InUse(ctx, q, n.ID)
			if err != nil {
				return err
			}
			if use {
				return ErrNodeInUse
			}
		}
		rows, err := q.TariffsLeftWithoutServers(ctx, db.TariffsLeftWithoutServersParams{NodeID: n.ID, InboundID: 0})
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			names := make([]string, 0, len(rows))
			for _, r := range rows {
				names = append(names, r.Name)
			}
			return &TariffServersError{Names: names}
		}
		if err := q.DeleteNode(ctx, n.ID); err != nil {
			return err
		}
		if rel.Unroute != nil {
			if err := rel.Unroute(ctx, q, n.ID); err != nil {
				return err
			}
		}
		return q.DeleteNodeStateOf(ctx, strconv.FormatInt(n.ID, 10))
	})
	return gone, err
}
