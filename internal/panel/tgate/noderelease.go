package tgate

import (
	"context"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
)

// NodeRelease is how the bot's route lets go of a node that is being removed: the route may
// name it (it goes to Telegram through it), and a removal that goes ahead clears that.
func NodeRelease(host domain.HostLookup) domain.NodeRelease {
	return domain.NodeRelease{
		Host: host,
		InUse: func(ctx context.Context, q *db.Queries, id int64) (bool, error) {
			route, _, err := settings.Get[Route](ctx, settings.New(q), KeyRoute)
			if err != nil {
				return false, err
			}
			return route.Mode == RouteNode && route.NodeID == id, nil
		},
		Unroute: func(ctx context.Context, q *db.Queries, id int64) error {
			set := settings.New(q)
			route, ok, err := settings.Get[Route](ctx, set, KeyRoute)
			if err != nil || !ok || route.NodeID != id {
				return err
			}
			route.NodeID = 0
			return settings.Set(ctx, set, KeyRoute, route)
		},
	}
}
