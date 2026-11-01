package botpanel

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/audit"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/model"
	"mikan/internal/panel/nodesync"
	"mikan/internal/panel/nodeupdate"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/subs"
	"mikan/internal/panel/tgate"
	"mikan/internal/panel/tgbot"
)

// The admin's servers. The live side (is the node answering, its version and load, a check, a
// speed test, a restart) is the node manager's, and an update is the update service's; both
// are the very ones the admin panel uses, handed in as the narrow interfaces below. Without
// them (a panel without nodes) a card shows what the database knows and the actions that need
// the node refuse.

// LiveNodes is what /admin needs of the node manager.
type LiveNodes interface {
	Health(id int64) (nodesync.HealthView, bool)
	CheckNow(ctx context.Context, id int64) (nodesync.HealthView, bool)
	SpeedTest(ctx context.Context, id int64) (nodeapi.SpeedTest, error)
	HostRestart(ctx context.Context, id int64) (nodeapi.RestartAccepted, error)
	Retire(ctx context.Context, id int64) error
	NodesChanged()
}

// NodeUpdater is what /admin needs of the node update service.
type NodeUpdater interface {
	Request(ctx context.Context, n db.Node) error
	Attempt(ctx context.Context, id int64) (nodeupdate.Attempt, bool)
}

// nodeDayHours is the window of the traffic a card shows.
const nodeDayHours = 24

// node writes what an admin changed on a server. Ids and numbers only.
func (a admin) nodeNote(ctx context.Context, by int64, action string, id int64, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	details["actor"] = actor(by)
	_ = audit.Write(ctx, a.l.Store.Q, a.now(), audit.Entry{
		Action: action, TargetType: "node", TargetID: strconv.FormatInt(id, 10), Details: details,
	})
}

// getNode reads a node, ErrNotFound when it is gone.
func (a admin) getNode(ctx context.Context, id int64) (db.Node, error) {
	n, err := a.l.Store.Q.GetNode(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Node{}, model.ErrNotFound
	}
	return n, err
}

// nodeView is a server's card as it is now.
func (a admin) nodeView(ctx context.Context, n db.Node) (tgbot.AdminNodeInfo, error) {
	host := n.PublicHost
	if host == "" {
		host = n.Domain
	}
	v := tgbot.AdminNodeInfo{
		AdminNode:  tgbot.AdminNode{ID: n.ID, Name: n.Name, Host: host, Enabled: n.Enabled == 1, Local: nodeupdate.Local(n)},
		PublicName: n.PublicName, Domain: n.Domain, Status: "unknown",
	}
	if n.ChannelMbps.Valid {
		v.ChannelMbps = n.ChannelMbps.Int64
	}
	inbounds, err := a.l.Store.Q.ListInbounds(ctx)
	if err != nil {
		return v, err
	}
	v.Inbounds = len(domain.NodeInbounds(inbounds, n.ID))
	if a.l.Nodes != nil {
		if hv, ok := a.l.Nodes.Health(n.ID); ok && !hv.CheckedAt.IsZero() {
			a.fillHealth(&v, hv)
		}
	}
	if v.Local {
		// The panel's own node is the panel: it is up when the panel answers.
		v.Online, v.Status = true, "ok"
	}
	if a.l.NodeUpdates != nil && !v.Local {
		if at, ok := a.l.NodeUpdates.Attempt(ctx, n.ID); ok && at.State == nodeupdate.Running {
			v.Updating = true
		}
	}
	rows, err := a.l.Store.Q.SumNodesTrafficHourly(ctx, a.now().Add(-nodeDayHours*3600*1e9).Unix()/3600)
	if err != nil {
		return v, err
	}
	for _, r := range rows {
		if r.NodeID == n.ID {
			v.Traffic24h = r.Up + r.Down
		}
	}
	return v, nil
}

func (a admin) fillHealth(v *tgbot.AdminNodeInfo, hv nodesync.HealthView) {
	if !hv.OK {
		v.Status, v.Error = "error", hv.Error
		if !hv.Since.IsZero() {
			v.ErrorSince = hv.Since.Unix()
		}
		return
	}
	v.Online, v.Status = true, "ok"
	v.Version, v.Conns = hv.Health.Version, hv.Health.Conns
	v.CPU, v.MemUsed, v.MemTotal = hv.Health.System.CPUPercent, hv.Health.System.MemUsed, hv.Health.System.MemTotal
	for _, l := range hv.Listeners {
		v.Listeners++
		if l.OK {
			v.ListenersOK++
		}
	}
	v.PanelVer = a.l.Version
	v.Behind = a.l.Version != "" && nodeupdate.Behind(a.l.Version, v.Version)
	v.CanUpdate = !v.Local && nodeupdate.CanUpdate(v.Version)
}

func (a admin) Node(ctx context.Context, by, id int64) (tgbot.AdminNodeInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	n, err := a.getNode(ctx, id)
	if err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	return a.nodeView(ctx, n)
}

func (a admin) SetNodeEnabled(ctx context.Context, by, id int64, on bool) (tgbot.AdminNodeInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	return a.patchNode(ctx, by, id, "tgadmin.node_enabled", domain.NodePatch{Enabled: &on}, map[string]any{"enabled": on})
}

// SetNodeField changes the name, the public name or the channel. The rules are the admin
// panel's: the name must fit the subscription's groups, the channel is 1..100000 Mbit/s.
func (a admin) SetNodeField(ctx context.Context, by, id int64, field, value string) (tgbot.AdminNodeInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	value = strings.TrimSpace(value)
	var p domain.NodePatch
	switch field {
	case tgbot.NodeFieldName:
		if value == "" || !cleanText(value) {
			return tgbot.AdminNodeInfo{}, model.ErrAdminInput
		}
		p.Name = &value
	case tgbot.NodeFieldPublic:
		if !cleanText(value) {
			return tgbot.AdminNodeInfo{}, model.ErrAdminInput
		}
		p.PublicName = &value
	case tgbot.NodeFieldChannel:
		mbps, err := strconv.ParseInt(value, 10, 64)
		if err != nil || mbps < 1 || mbps > 100000 {
			return tgbot.AdminNodeInfo{}, model.ErrAdminInput
		}
		p.ChannelMbps = &mbps
	default:
		return tgbot.AdminNodeInfo{}, model.ErrAdminInput
	}
	return a.patchNode(ctx, by, id, "tgadmin.node_"+field, p, map[string]any{"field": field})
}

// cleanText: a name of a server has no control characters and fits MaxNodeText.
func cleanText(s string) bool {
	if utf8.RuneCountInString(s) > tgbot.MaxNodeText {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// patchNode applies p the way the admin panel does and says what the rules refused.
func (a admin) patchNode(ctx context.Context, by, id int64, action string, p domain.NodePatch, details map[string]any) (tgbot.AdminNodeInfo, error) {
	g, err := subs.LoadGroups(ctx, a.l.Settings)
	if err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	p.Rules = domain.NodeRules{Name: g.CheckName}
	n, err := domain.UpdateNode(ctx, a.l.Store, id, p, a.now())
	if err != nil {
		return tgbot.AdminNodeInfo{}, nodeRefusal(err)
	}
	if a.l.Nodes != nil {
		a.l.Nodes.NodesChanged()
	}
	if a.l.SlotsChanged != nil {
		a.l.SlotsChanged()
	}
	if a.l.WakeNodeCerts != nil {
		a.l.WakeNodeCerts()
	}
	a.nodeNote(ctx, by, action, id, details)
	return a.nodeView(ctx, n)
}

// nodeRefusal turns what the node rules refused into the bot's error.
func nodeRefusal(err error) error {
	var name *domain.NameError
	switch {
	case errors.Is(err, domain.ErrUnknownNode):
		return model.ErrNotFound
	case errors.Is(err, domain.ErrChannelRequired), errors.As(err, &name):
		return model.ErrAdminInput
	}
	return err
}

func (a admin) CheckNode(ctx context.Context, by, id int64) (tgbot.AdminNodeInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	n, err := a.getNode(ctx, id)
	if err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	if a.l.Nodes != nil && !nodeupdate.Local(n) {
		a.l.Nodes.CheckNow(ctx, id)
	}
	a.nodeNote(ctx, by, "tgadmin.node_check", id, nil)
	return a.nodeView(ctx, n)
}

func (a admin) SpeedNode(ctx context.Context, by, id int64) (tgbot.AdminSpeed, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminSpeed{}, err
	}
	if _, err := a.getNode(ctx, id); err != nil {
		return tgbot.AdminSpeed{}, err
	}
	if a.l.Nodes == nil {
		return tgbot.AdminSpeed{}, model.ErrNodeOffline
	}
	st, err := a.l.Nodes.SpeedTest(ctx, id)
	if err != nil {
		return tgbot.AdminSpeed{}, nodeFailure(err)
	}
	a.nodeNote(ctx, by, "tgadmin.node_speed", id, nil)
	return tgbot.AdminSpeed{PingMs: st.PingMs, LossPct: st.LossPct, DownBps: st.DownBps, UpBps: st.UpBps, Error: st.Error}, nil
}

func (a admin) UpdateNode(ctx context.Context, by, id int64) (tgbot.AdminNodeInfo, error) {
	if err := a.check(ctx, by); err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	n, err := a.getNode(ctx, id)
	if err != nil {
		return tgbot.AdminNodeInfo{}, err
	}
	if a.l.NodeUpdates == nil {
		return tgbot.AdminNodeInfo{}, model.ErrNodeOffline
	}
	if err := a.l.NodeUpdates.Request(ctx, n); err != nil {
		return tgbot.AdminNodeInfo{}, updateRefusal(err)
	}
	a.nodeNote(ctx, by, "tgadmin.node_update", id, map[string]any{"version": a.l.Version})
	return a.nodeView(ctx, n)
}

// updateRefusal says why the update service would not start.
func updateRefusal(err error) error {
	switch {
	case errors.Is(err, nodeupdate.ErrLocal):
		return model.ErrNodeLocal
	case errors.Is(err, nodeupdate.ErrOffline):
		return model.ErrNodeOffline
	case errors.Is(err, nodeupdate.ErrNotBehind):
		return model.ErrNodeNotBehind
	case errors.Is(err, nodeupdate.ErrBusy):
		return model.ErrNodeUpdating
	case errors.Is(err, nodeupdate.ErrUnsupported), errors.Is(err, nodeupdate.ErrNotRelease):
		return model.ErrNodeCannot
	}
	return nodeFailure(err)
}

// nodeFailure: the node or the network failed; the cause stays in the log.
func nodeFailure(err error) error {
	if errors.Is(err, nodeapi.ErrUnavailable) {
		return model.ErrNodeOffline
	}
	return model.ErrNodeFailed
}

func (a admin) RestartNode(ctx context.Context, by, id int64) error {
	if err := a.check(ctx, by); err != nil {
		return err
	}
	n, err := a.getNode(ctx, id)
	if err != nil {
		return err
	}
	if nodeupdate.Local(n) {
		return model.ErrNodeLocal
	}
	if a.l.Nodes == nil {
		return model.ErrNodeOffline
	}
	if _, err := a.l.Nodes.HostRestart(ctx, id); err != nil {
		return nodeFailure(err)
	}
	a.nodeNote(ctx, by, "tgadmin.node_restart", id, nil)
	return nil
}

// DeleteNode removes a remote server by the very function the admin panel uses
// (domain.DeleteNode): what went out through it goes another way, the bot's route lets go of
// it, and a server the bot needs or a tariff would be left without is refused. The panel's own
// node is never removed.
func (a admin) DeleteNode(ctx context.Context, by, id int64) error {
	if err := a.check(ctx, by); err != nil {
		return err
	}
	var host domain.HostLookup
	if a.l.Nodes != nil {
		host = func(id int64) *nodeapi.HostPorts {
			if hv, ok := a.l.Nodes.Health(id); ok {
				return hv.HostPorts()
			}
			return nil
		}
	}
	gone, err := domain.DeleteNode(ctx, a.l.Store, id, a.now(), tgate.NodeRelease(host))
	if err != nil {
		return deleteRefusal(err)
	}
	if a.l.Nodes != nil {
		// Best effort: an unreachable node keeps serving until it is reinstalled.
		_ = a.l.Nodes.Retire(ctx, id)
		a.l.Nodes.NodesChanged()
	}
	if a.l.ForgetNode != nil {
		_ = a.l.ForgetNode(id)
	}
	if a.l.SlotsChanged != nil {
		a.l.SlotsChanged()
	}
	a.nodeNote(ctx, by, "tgadmin.node_delete", id, map[string]any{"name": gone.Node.Name, "exits_moved": len(gone.Moved)})
	return nil
}

// deleteRefusal is what the removal refused, in the bot's errors.
func deleteRefusal(err error) error {
	var left *domain.TariffServersError
	switch {
	case errors.Is(err, domain.ErrUnknownNode):
		return model.ErrNotFound
	case errors.Is(err, domain.ErrNodeLocal):
		return model.ErrNodeLocal
	case errors.Is(err, domain.ErrNodeInUse), errors.As(err, &left):
		return model.ErrNodeInUse
	}
	return err
}
