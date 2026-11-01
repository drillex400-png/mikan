package botpanel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mikan/internal/panel/model"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/tgate"
	"mikan/internal/panel/tgbot"
)

const adminTg = 424242

// setAdmins saves the bot's setup with the given admins.
func setAdmins(t *testing.T, e *env, ids ...int64) {
	t.Helper()
	cfg := tgbot.Default("ru")
	cfg.AdminIDs = ids
	if err := settings.Set(e.ctx, e.set, tgate.KeyConfig, cfg); err != nil {
		t.Fatal(err)
	}
}

func auditActions(t *testing.T, e *env) []string {
	t.Helper()
	rows, err := e.st.Q.ListAudit(e.ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Action)
	}
	return out
}

// Whoever is not in the list of admins gets nothing from any admin call and changes nothing,
// whatever the bot decided to show.
func TestAdminRefusesAStranger(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	for _, by := range []int64{0, 1, -adminTg, adminTg + 1} {
		if _, err := a.Overview(ctx, by); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Overview by %d: %v", by, err)
		}
		if _, _, err := a.Users(ctx, by, "", 0); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Users by %d: %v", by, err)
		}
		if _, err := a.User(ctx, by, e.user.ID); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("User by %d: %v", by, err)
		}
		if _, err := a.GrantDays(ctx, by, e.user.ID, 30); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("GrantDays by %d: %v", by, err)
		}
		if _, err := a.SetDisabled(ctx, by, e.user.ID, true); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("SetDisabled by %d: %v", by, err)
		}
		if _, err := a.Nodes(ctx, by); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Nodes by %d: %v", by, err)
		}
		if _, err := a.Payments(ctx, by); !errors.Is(err, model.ErrNotAdmin) {
			t.Errorf("Payments by %d: %v", by, err)
		}
	}
	after, err := e.st.Q.GetUser(ctx, e.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ExpiresAt != e.user.ExpiresAt || after.Status != e.user.Status {
		t.Fatalf("a stranger changed the subscription: %+v -> %+v", e.user, after)
	}
	if got := auditActions(t, e); len(got) != 0 {
		t.Fatalf("a refused call was written to the log: %v", got)
	}
}

// With no admins set, nobody is one.
func TestAdminEmptyListAdmitsNobody(t *testing.T) {
	e := setup(t)
	if _, err := e.local.Admin().Overview(context.Background(), adminTg); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("no admins set: %v", err)
	}
}

// An admin extends and blocks; each change is written to the audit log with who made it, and
// carries no link or token.
func TestAdminChangesAreAudited(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()

	before := e.user.ExpiresAt.Int64
	v, err := a.GrantDays(ctx, adminTg, e.user.ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	if v.ExpiresAt <= before {
		t.Fatalf("the term did not grow: %d -> %d", before, v.ExpiresAt)
	}
	if v, err = a.SetDisabled(ctx, adminTg, e.user.ID, true); err != nil || v.State != "disabled" {
		t.Fatalf("block: %+v %v", v, err)
	}
	if v, err = a.SetDisabled(ctx, adminTg, e.user.ID, false); err != nil || v.State == "disabled" {
		t.Fatalf("unblock: %+v %v", v, err)
	}
	got := strings.Join(auditActions(t, e), ",")
	for _, want := range []string{"tgadmin.extend", "tgadmin.disable"} {
		if !strings.Contains(got, want) {
			t.Errorf("the log has no %s: %s", want, got)
		}
	}
	rows, _ := e.st.Q.ListAudit(ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 100})
	for _, r := range rows {
		if strings.HasPrefix(r.Action, "tgadmin.") {
			if !r.Details.Valid || !strings.Contains(r.Details.String, "tg:424242") {
				t.Errorf("%s does not name who made it: %v", r.Action, r.Details)
			}
			if strings.Contains(r.Details.String, e.user.SubToken) {
				t.Errorf("%s carries the subscription token", r.Action)
			}
		}
	}
}

// The bounds of one extension hold, and an unknown subscription is not found.
func TestAdminBounds(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	for _, days := range []int64{0, -5, tgbot.MaxAdminDays + 1} {
		if _, err := a.GrantDays(ctx, adminTg, e.user.ID, days); !errors.Is(err, model.ErrAdminInput) {
			t.Errorf("%d days: %v", days, err)
		}
	}
	if _, err := a.GrantDays(ctx, adminTg, e.user.ID+9999, 5); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("an unknown subscription: %v", err)
	}
	if _, err := a.User(ctx, adminTg, e.user.ID+9999); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("an unknown subscription card: %v", err)
	}
}

// A removed admin loses access on the next call, with no restart.
func TestAdminRemovedLosesAccessAtOnce(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	if _, err := a.Overview(ctx, adminTg); err != nil {
		t.Fatal(err)
	}
	setAdmins(t, e, 7)
	if _, err := a.Overview(ctx, adminTg); !errors.Is(err, model.ErrNotAdmin) {
		t.Fatalf("a removed admin: %v", err)
	}
}

// The overview and the list work, and the list pages.
func TestAdminReads(t *testing.T) {
	e := setup(t)
	setAdmins(t, e, adminTg)
	a := e.local.Admin()
	ctx := context.Background()
	o, err := a.Overview(ctx, adminTg)
	if err != nil || o.Users < 1 {
		t.Fatalf("overview: %+v %v", o, err)
	}
	us, more, err := a.Users(ctx, adminTg, "", 0)
	if err != nil || len(us) == 0 || more {
		t.Fatalf("users: %d %v %v", len(us), more, err)
	}
	if none, _, _ := a.Users(ctx, adminTg, "no-such-name-zzz", 0); len(none) != 0 {
		t.Fatalf("a search that matches nobody found %d", len(none))
	}
	if _, err := a.Nodes(ctx, adminTg); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Payments(ctx, adminTg); err != nil {
		t.Fatal(err)
	}
}
