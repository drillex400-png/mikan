package tgbot

import "testing"

func TestAdminIDsValidate(t *testing.T) {
	good := Default("ru")
	good.AdminIDs = []int64{5, 7, 5}
	if err := good.Validate(); err != nil {
		t.Fatalf("good list: %v", err)
	}
	if len(good.AdminIDs) != 2 {
		t.Fatalf("a repeat was kept: %v", good.AdminIDs)
	}
	for name, ids := range map[string][]int64{
		"zero":     {0},
		"negative": {-1001234567890},
		"too many": func() []int64 {
			l := make([]int64, MaxAdmins+1)
			for i := range l {
				l[i] = int64(i + 1)
			}
			return l
		}(),
	} {
		c := Default("ru")
		c.AdminIDs = ids
		if err := c.Validate(); err != ErrAdminIDs {
			t.Errorf("%s: got %v, want ErrAdminIDs", name, err)
		}
	}
}

func TestIsAdmin(t *testing.T) {
	c := Config{AdminIDs: []int64{42}}
	if !c.IsAdmin(42) || c.IsAdmin(43) || c.IsAdmin(0) || c.IsAdmin(-42) {
		t.Fatal("IsAdmin lets the wrong account in")
	}
	if (Config{}).IsAdmin(42) {
		t.Fatal("an empty list admits somebody")
	}
}
