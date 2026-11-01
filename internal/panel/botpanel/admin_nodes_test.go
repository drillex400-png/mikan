package botpanel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/model"
	"mikan/internal/panel/nodesync"
	"mikan/internal/panel/nodeupdate"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgbot"
)

// liveNodes is the node manager as /admin sees it: what each node answers, and what was asked.
type liveNodes struct {
	mu      sync.Mutex
	health  map[int64]nodesync.HealthView
	calls   []string
	speed   nodeapi.SpeedTest
	failure error
}

func (l *liveNodes) rec(s string) { l.mu.Lock(); l.calls = append(l.calls, s); l.mu.Unlock() }
func (l *liveNodes) Health(id int64) (nodesync.HealthView, bool) {
	hv, ok := l.health[id]
	return hv, ok
}
func (l *liveNodes) CheckNow(_ context.Context, id int64) (nodesync.HealthView, bool) {
	l.rec("check")
	hv, ok := l.health[id]
	return hv, ok
}
func (l *liveNodes) SpeedTest(context.Context, int64) (nodeapi.SpeedTest, error) {
	l.rec("speed")
	return l.speed, l.failure
}
func (l *liveNodes) HostRestart(context.Context, int64) (nodeapi.RestartAccepted, error) {
	l.rec("restart")
	return nodeapi.RestartAccepted{ID: "r1"}, l.failure
}
func (l *liveNodes) Retire(context.Context, int64) error { l.rec("retire"); return nil }
func (l *liveNodes) NodesChanged()                       { l.rec("changed") }

func (l *liveNodes) called(s string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.calls {
		if c == s {
			return true
		}
	}
	return false
}

// updater is the node update service: it answers what Request is told to.
type updater struct {
	err   error
	asked []int64
}

func (u *updater) Request(_ context.Context, n db.Node) error {
	u.asked = append(u.asked, n.ID)
	return u.err
}
func (u *updater) Attempt(context.Context, int64) (nodeupdate.Attempt, bool) {
	return nodeupdate.Attempt{}, false
}

// addNode makes a remote node and says it answers with the given version.
func addNode(t *testing.T, e *env, live *liveNodes, name, version string) db.Node {
	t.Helper()
	n, err := e.st.Q.CreateNode(e.ctx, db.CreateNodeParams{Name: name, Address: name + ".example:25305", PublicHost: "203.0.113.7", CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if live.health == nil {
		live.health = map[int64]nodesync.HealthView{}
	}
	hv := nodesync.HealthView{OK: true, CheckedAt: e.now}
	hv.Health.Version = version
	hv.Health.Conns = 5
	hv.Health.System.CPUPercent = 40
	hv.Health.System.MemUsed, hv.Health.System.MemTotal = 1<<30, 4<<30
	live.health[n.ID] = hv
	return n
}

func serversEnv(t *testing.T) (*env, *liveNodes, *updater) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	live, up := &liveNodes{}, &updater{}
	e.local.Nodes, e.local.NodeUpdates, e.local.Version = live, up, "0.5.2.1-rc.12"
	return e, live, up
}

// A stranger gets nothing from any call about the servers and changes nothing.
func TestServersRefuseAStranger(t *testing.T) {
	e, live, up := serversEnv(t)
	n := addNode(t, e, live, "nl", "0.5.2.1-rc.10")
	a := e.local.Admin()
	for _, by := range []int64{0, 1, adminTg + 1} {
		refused := map[string]error{}
		_, refused["Node"] = a.Node(e.ctx, by, n.ID)
		_, refused["SetNodeEnabled"] = a.SetNodeEnabled(e.ctx, by, n.ID, false)
		_, refused["SetNodeField"] = a.SetNodeField(e.ctx, by, n.ID, tgbot.NodeFieldName, "x")
		_, refused["CheckNode"] = a.CheckNode(e.ctx, by, n.ID)
		_, refused["SpeedNode"] = a.SpeedNode(e.ctx, by, n.ID)
		_, refused["UpdateNode"] = a.UpdateNode(e.ctx, by, n.ID)
		refused["RestartNode"] = a.RestartNode(e.ctx, by, n.ID)
		refused["DeleteNode"] = a.DeleteNode(e.ctx, by, n.ID)
		for name, err := range refused {
			if !errors.Is(err, model.ErrNotAdmin) {
				t.Errorf("%s by %d: %v", name, by, err)
			}
		}
	}
	got, err := e.st.Q.GetNode(e.ctx, n.ID)
	if err != nil || got.Name != "nl" || got.Enabled != 1 {
		t.Fatalf("a stranger changed the node: %+v %v", got, err)
	}
	if len(live.calls) != 0 || len(up.asked) != 0 {
		t.Fatalf("a stranger reached the node: %v %v", live.calls, up.asked)
	}
	if got := auditActions(t, e); len(got) != 0 {
		t.Fatalf("a refused call was written to the log: %v", got)
	}
}

// The card tells how the node is now: online, the version and whether it is behind, the load.
func TestServerCard(t *testing.T) {
	e, live, _ := serversEnv(t)
	n := addNode(t, e, live, "nl", "0.5.2.1-rc.10")
	v, err := e.local.Admin().Node(e.ctx, adminTg, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Online || v.Status != "ok" || v.Version != "0.5.2.1-rc.10" || !v.Behind || v.Conns != 5 || v.CPU != 40 || v.MemTotal != 4<<30 {
		t.Fatalf("card: %+v", v)
	}
	if v.Local {
		t.Fatal("a remote node is shown as the panel's own")
	}
	// A node that does not answer is shown with why, and is not online.
	hv := live.health[n.ID]
	hv.OK, hv.Error = false, "timeout"
	live.health[n.ID] = hv
	if v, _ = e.local.Admin().Node(e.ctx, adminTg, n.ID); v.Online || v.Status != "error" || v.Error != "timeout" {
		t.Fatalf("a node that is down: %+v", v)
	}
	if _, err := e.local.Admin().Node(e.ctx, adminTg, 987654); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("an unknown node: %v", err)
	}
	// Without the node manager the card shows what the database knows.
	e.local.Nodes = nil
	if v, err = e.local.Admin().Node(e.ctx, adminTg, n.ID); err != nil || v.Name != "nl" || v.Online {
		t.Fatalf("a panel without nodes: %+v %v", v, err)
	}
}

// Turning a node off and on, and the name, the public name and the channel: changed through the
// rules of the admin panel, each change written to the log under the admin's account.
func TestServerChanges(t *testing.T) {
	e, live, _ := serversEnv(t)
	n := addNode(t, e, live, "nl", "0.5.2.1-rc.12")
	a := e.local.Admin()
	if v, err := a.SetNodeEnabled(e.ctx, adminTg, n.ID, false); err != nil || v.Enabled {
		t.Fatalf("off: %+v %v", v, err)
	}
	if v, err := a.SetNodeEnabled(e.ctx, adminTg, n.ID, true); err != nil || !v.Enabled {
		t.Fatalf("on: %+v %v", v, err)
	}
	if v, err := a.SetNodeField(e.ctx, adminTg, n.ID, tgbot.NodeFieldName, "  Berlin  "); err != nil || v.Name != "Berlin" {
		t.Fatalf("name: %+v %v", v, err)
	}
	if v, err := a.SetNodeField(e.ctx, adminTg, n.ID, tgbot.NodeFieldPublic, "Berlin public"); err != nil || v.PublicName != "Berlin public" {
		t.Fatalf("public: %+v %v", v, err)
	}
	if v, err := a.SetNodeField(e.ctx, adminTg, n.ID, tgbot.NodeFieldChannel, "500"); err != nil || v.ChannelMbps != 500 {
		t.Fatalf("channel: %+v %v", v, err)
	}
	if !live.called("changed") {
		t.Error("the syncer was not told the nodes changed")
	}
	got := strings.Join(auditActions(t, e), ",")
	for _, want := range []string{"tgadmin.node_enabled", "tgadmin.node_n", "tgadmin.node_p", "tgadmin.node_c"} {
		if !strings.Contains(got, want) {
			t.Errorf("the log has no %s: %s", want, got)
		}
	}
	rows, _ := e.st.Q.ListAudit(e.ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 100})
	for _, r := range rows {
		if strings.HasPrefix(r.Action, "tgadmin.node") && (!r.Details.Valid || !strings.Contains(r.Details.String, "tg:424242")) {
			t.Errorf("%s does not name who made it: %v", r.Action, r.Details)
		}
		if r.Action != "" && strings.HasPrefix(r.Action, "tgadmin.node") && r.TargetType.String != "node" {
			t.Errorf("%s is about a %q", r.Action, r.TargetType.String)
		}
	}
}

// What is out of bounds or refused by the rules comes back as the bot's input error, and the
// node stays as it was.
func TestServerChangeBounds(t *testing.T) {
	e, live, _ := serversEnv(t)
	n := addNode(t, e, live, "nl", "0.5.2.1-rc.12")
	other := addNode(t, e, live, "de", "0.5.2.1-rc.12")
	a := e.local.Admin()
	for name, c := range map[string][2]string{
		"empty name": {tgbot.NodeFieldName, ""}, "control character": {tgbot.NodeFieldName, "a\x00b"},
		"long name":   {tgbot.NodeFieldName, strings.Repeat("я", tgbot.MaxNodeText+1)},
		"name in use": {tgbot.NodeFieldName, other.Name}, "zero channel": {tgbot.NodeFieldChannel, "0"},
		"huge channel": {tgbot.NodeFieldChannel, "100001"}, "text channel": {tgbot.NodeFieldChannel, "fast"},
		"unknown field": {"x", "y"},
	} {
		if _, err := a.SetNodeField(e.ctx, adminTg, n.ID, c[0], c[1]); !errors.Is(err, model.ErrAdminInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := a.SetNodeField(e.ctx, adminTg, 987654, tgbot.NodeFieldName, "x"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("unknown node: %v", err)
	}
	if got, _ := e.st.Q.GetNode(e.ctx, n.ID); got.Name != "nl" {
		t.Errorf("a refused change landed: %q", got.Name)
	}
}

// Checking, a speed test and a restart go to the node; a failure is told as the node's, not as
// the panel's.
func TestServerActions(t *testing.T) {
	e, live, _ := serversEnv(t)
	n := addNode(t, e, live, "nl", "0.5.2.1-rc.12")
	live.speed = nodeapi.SpeedTest{PingMs: 11, LossPct: 0.5, DownBps: 90e6, UpBps: 40e6}
	a := e.local.Admin()
	if _, err := a.CheckNode(e.ctx, adminTg, n.ID); err != nil || !live.called("check") {
		t.Fatalf("check: %v %v", err, live.calls)
	}
	sp, err := a.SpeedNode(e.ctx, adminTg, n.ID)
	if err != nil || sp.DownBps != 90e6 || sp.PingMs != 11 {
		t.Fatalf("speed: %+v %v", sp, err)
	}
	if err := a.RestartNode(e.ctx, adminTg, n.ID); err != nil || !live.called("restart") {
		t.Fatalf("restart: %v %v", err, live.calls)
	}
	live.failure = errors.New("boom: secret detail")
	if err := a.RestartNode(e.ctx, adminTg, n.ID); !errors.Is(err, model.ErrNodeFailed) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("a failed restart: %v", err)
	}
	live.failure = nodeapi.ErrUnavailable
	if _, err := a.SpeedNode(e.ctx, adminTg, n.ID); !errors.Is(err, model.ErrNodeOffline) {
		t.Fatalf("an unreachable node: %v", err)
	}
	got := strings.Join(auditActions(t, e), ",")
	for _, want := range []string{"tgadmin.node_check", "tgadmin.node_speed", "tgadmin.node_restart"} {
		if !strings.Contains(got, want) {
			t.Errorf("the log has no %s: %s", want, got)
		}
	}
}

// The update service's refusals come back as the bot's, and only an accepted one is logged.
func TestServerUpdate(t *testing.T) {
	e, live, up := serversEnv(t)
	n := addNode(t, e, live, "nl", "0.5.2.1-rc.10")
	a := e.local.Admin()
	if _, err := a.UpdateNode(e.ctx, adminTg, n.ID); err != nil || len(up.asked) != 1 || up.asked[0] != n.ID {
		t.Fatalf("update: %v %v", err, up.asked)
	}
	for refusal, want := range map[error]error{
		nodeupdate.ErrLocal: model.ErrNodeLocal, nodeupdate.ErrOffline: model.ErrNodeOffline, nodeupdate.ErrNotBehind: model.ErrNodeNotBehind,
		nodeupdate.ErrBusy: model.ErrNodeUpdating, nodeupdate.ErrUnsupported: model.ErrNodeCannot, nodeupdate.ErrNotRelease: model.ErrNodeCannot,
		errors.New("other"): model.ErrNodeFailed,
	} {
		up.err = refusal
		if _, err := a.UpdateNode(e.ctx, adminTg, n.ID); !errors.Is(err, want) {
			t.Errorf("%v: got %v, want %v", refusal, err, want)
		}
	}
	if n := strings.Count(strings.Join(auditActions(t, e), ","), "tgadmin.node_update"); n != 1 {
		t.Errorf("%d updates in the log, want only the accepted one", n)
	}
	e.local.NodeUpdates = nil
	up.err = nil
	if _, err := a.UpdateNode(e.ctx, adminTg, n.ID); !errors.Is(err, model.ErrNodeOffline) {
		t.Errorf("a panel without the update service: %v", err)
	}
}

// The panel's own node follows the panel: it cannot be restarted or removed from the bot.
func TestServerLocalNodeIsKept(t *testing.T) {
	e, live, _ := serversEnv(t)
	nodes, err := e.st.Q.ListNodes(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var local db.Node
	for _, n := range nodes {
		if n.Address == "" {
			local = n
		}
	}
	if local.ID == 0 {
		t.Skip("no local node in this setup")
	}
	a := e.local.Admin()
	if err := a.RestartNode(e.ctx, adminTg, local.ID); !errors.Is(err, model.ErrNodeLocal) {
		t.Errorf("restart: %v", err)
	}
	if err := a.DeleteNode(e.ctx, adminTg, local.ID); !errors.Is(err, model.ErrNodeLocal) {
		t.Errorf("delete: %v", err)
	}
	if _, err := e.st.Q.GetNode(e.ctx, local.ID); err != nil {
		t.Errorf("the panel's own node is gone: %v", err)
	}
	if live.called("retire") || live.called("restart") {
		t.Errorf("the local node was reached: %v", live.calls)
	}
}

// Removing a node goes through the rules of the admin panel: the row goes, the node is told to
// drop its users, its files are cleared; a node that is not there is not found.
func TestServerDelete(t *testing.T) {
	e, live, _ := serversEnv(t)
	n := addNode(t, e, live, "nl", "0.5.2.1-rc.12")
	var forgot []int64
	e.local.ForgetNode = func(id int64) error { forgot = append(forgot, id); return nil }
	a := e.local.Admin()
	if err := a.DeleteNode(e.ctx, adminTg, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Q.GetNode(e.ctx, n.ID); err == nil {
		t.Fatal("the node is still there")
	}
	if !live.called("retire") || len(forgot) != 1 || forgot[0] != n.ID {
		t.Errorf("not cleaned up: %v %v", live.calls, forgot)
	}
	if !strings.Contains(strings.Join(auditActions(t, e), ","), "tgadmin.node_delete") {
		t.Error("the removal is not in the log")
	}
	if err := a.DeleteNode(e.ctx, adminTg, n.ID); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("a second removal: %v", err)
	}
}

// The list shows a server the way its card does: a node that answers is online, one that does
// not is not, a node never checked is not claimed to be up, and the panel's own is up.
func TestServerListMatchesTheCard(t *testing.T) {
	e, live, _ := serversEnv(t)
	up := addNode(t, e, live, "up", "0.5.2.1-rc.10")
	down := addNode(t, e, live, "down", "0.5.2.1-rc.12")
	hv := live.health[down.ID]
	hv.OK, hv.Error = false, "timeout"
	live.health[down.ID] = hv
	fresh, err := e.st.Q.CreateNode(e.ctx, db.CreateNodeParams{Name: "fresh", Address: "fresh.example:25305", PublicHost: "203.0.113.8", CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	list, err := e.local.Admin().Nodes(e.ctx, adminTg)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]tgbot.AdminNode{}
	for _, n := range list {
		seen[n.ID] = n
	}
	if n := seen[up.ID]; !n.Online || n.Version != "0.5.2.1-rc.10" || !n.Behind {
		t.Errorf("a node that answers: %+v", n)
	}
	if n := seen[down.ID]; n.Online {
		t.Errorf("a node that does not answer is online: %+v", n)
	}
	if n := seen[fresh.ID]; n.Online {
		t.Errorf("a node never checked is online: %+v", n)
	}
	for _, n := range list {
		if n.Local && !n.Online {
			t.Errorf("the panel's own node is down: %+v", n)
		}
		card, err := e.local.Admin().Node(e.ctx, adminTg, n.ID)
		if err != nil {
			t.Fatal(err)
		}
		if card.Online != n.Online {
			t.Errorf("%s: the list says online=%v, the card %v", n.Name, n.Online, card.Online)
		}
	}
}
