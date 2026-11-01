package node

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/component/geodata/router"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"
	"google.golang.org/protobuf/proto"

	"mikan/internal/nodeapi"
	mproto "mikan/internal/proto"
)

// testGeoIP and testGeoSite are real geoip.dat and geosite.dat, as small as they can be.
func testGeoIP(t *testing.T, country string) []byte {
	t.Helper()
	ip := netip.MustParseAddr("77.88.8.8").As4()
	b, err := proto.Marshal(&router.GeoIPList{Entry: []*router.GeoIP{{CountryCode: strings.ToUpper(country), Cidr: []*router.CIDR{{Ip: ip[:], Prefix: 24}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testGeoSite(t *testing.T, list string) []byte {
	t.Helper()
	b, err := proto.Marshal(&router.GeoSiteList{Entry: []*router.GeoSite{{CountryCode: strings.ToUpper(list),
		Domain: []*router.Domain{{Type: router.Domain_Domain, Value: "yandex.ru"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func geoEngine(t *testing.T) *Engine {
	t.Helper()
	geoOK.Store(false)
	t.Cleanup(func() { geoOK.Store(false) })
	return &Engine{home: t.TempDir()}
}

// A database is kept only when it reads and holds the entry the mode asks for; the mode
// is ready when both are in place, and not before.
func TestPutGeodata(t *testing.T) {
	e := geoEngine(t)

	for name, data := range map[string][]byte{
		GeoIP:   []byte("not a database"),
		GeoSite: nil,
	} {
		if err := e.PutGeodata(name, data); !errors.Is(err, ErrGeodataBad) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// It parses, but has no Russia in it: it would match nothing.
	if err := e.PutGeodata(GeoIP, testGeoIP(t, "us")); !errors.Is(err, ErrGeodataBad) {
		t.Fatalf("a geoip without ru: %v", err)
	}
	if err := e.PutGeodata(GeoSite, testGeoSite(t, "geolocation-!cn")); !errors.Is(err, ErrGeodataBad) {
		t.Fatalf("a geosite without category-ru: %v", err)
	}
	// The geosite is not a geoip, nor the reverse.
	if err := e.PutGeodata(GeoIP, testGeoSite(t, "category-ru")); !errors.Is(err, ErrGeodataBad) {
		t.Fatalf("a geosite sent as geoip: %v", err)
	}
	if err := e.PutGeodata("other", testGeoIP(t, "ru")); !errors.Is(err, ErrGeodataName) {
		t.Fatalf("unknown name: %v", err)
	}
	if err := e.PutGeodata(GeoIP, make([]byte, MaxGeodata+1)); !errors.Is(err, ErrGeodataBad) {
		t.Fatalf("too large: %v", err)
	}
	if ip, site := e.GeodataStatus(); ip || site || geoOK.Load() {
		t.Fatal("something was kept that should not have been")
	}

	if err := e.PutGeodata(GeoIP, testGeoIP(t, "ru")); err != nil {
		t.Fatal(err)
	}
	if geoOK.Load() {
		t.Fatal("ready with only one database")
	}
	if err := e.PutGeodata(GeoSite, testGeoSite(t, "category-ru")); err != nil {
		t.Fatal(err)
	}
	if !geoOK.Load() {
		t.Fatal("not ready with both")
	}
	if ip, site := e.GeodataStatus(); !ip || !site {
		t.Fatalf("status %v %v", ip, site)
	}

	// A bad file sent later leaves the good one, and the node ready.
	if err := e.PutGeodata(GeoIP, []byte("garbage")); !errors.Is(err, ErrGeodataBad) {
		t.Fatal(err)
	}
	if !geoOK.Load() {
		t.Fatal("a bad file took the mode down")
	}
	if b, _ := os.ReadFile(filepath.Join(e.home, geoIPFile)); string(b) != string(testGeoIP(t, "ru")) {
		t.Fatal("a bad file replaced the good one")
	}
}

// After a restart the files on disk are read again before the mode counts on them; a file
// that went bad in between takes the mode down instead of the node.
func TestLoadGeoAtStart(t *testing.T) {
	e := geoEngine(t)
	if err := e.PutGeodata(GeoIP, testGeoIP(t, "ru")); err != nil {
		t.Fatal(err)
	}
	if err := e.PutGeodata(GeoSite, testGeoSite(t, "category-ru")); err != nil {
		t.Fatal(err)
	}
	geoOK.Store(false)
	e.loadGeo()
	if !geoOK.Load() {
		t.Fatal("good files were not taken at start")
	}
	if err := os.WriteFile(filepath.Join(e.home, geoSiteFile), []byte("torn"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.loadGeo()
	if geoOK.Load() {
		t.Fatal("a torn file was trusted")
	}
	if err := os.Remove(filepath.Join(e.home, geoIPFile)); err != nil {
		t.Fatal(err)
	}
	e.loadGeo()
	if geoOK.Load() {
		t.Fatal("a missing file was trusted")
	}
}

func ruState(t *testing.T) nodeapi.DesiredState {
	st := cascadeState(t)
	st.Exits[0].RUDirect = []string{"in-anytls", "in-gone"}
	return st
}

func ruRules(st nodeapi.DesiredState, geo bool) []string { return exitRules(st, geo) }

// With the databases the Russian traffic of the chosen inbound leaves the node, and only
// then the rest goes to the exit; an inbound that is not chosen goes whole.
func TestExitRulesRUDirect(t *testing.T) {
	st := ruState(t)
	got := strings.Join(ruRules(st, true), "\n")
	want := []string{
		"AND,((IN-NAME,in-anytls),(GEOSITE,category-ru)),DIRECT",
		"AND,((IN-NAME,in-anytls),(GEOIP,ru,no-resolve)),DIRECT",
		"IN-NAME,in-anytls,NODE-3",
	}
	idx := -1
	for _, w := range want {
		i := strings.Index(got, w)
		if i < 0 || i < idx {
			t.Fatalf("%q missing or out of order in:\n%s", w, got)
		}
		idx = i
	}
	if strings.Contains(got, "IN-NAME,"+nodeapi.RelayListener+",NODE-3") == false {
		t.Fatal("the relay listener is not sent on")
	}
	// Not named in RUDirect: no exception, the whole of it goes on.
	if strings.Contains(got, "(IN-NAME,"+nodeapi.RelayListener+")") {
		t.Fatalf("the relay got the exception:\n%s", got)
	}
	// A listener that does not exist is named by nothing.
	if strings.Contains(got, "in-gone") {
		t.Fatalf("a missing listener in the rules:\n%s", got)
	}
}

// Without the databases the mode is off: the same rules as before it, whole to the exit.
func TestExitRulesRUDirectNeedsGeodata(t *testing.T) {
	st := ruState(t)
	got := ruRules(st, false)
	for _, r := range got {
		if strings.Contains(r, "GEO") || strings.HasPrefix(r, "AND,") {
			t.Fatalf("a rule names a database the node does not have: %s", r)
		}
	}
	plain := cascadeState(t)
	if strings.Join(got, "|") != strings.Join(exitRules(plain, true), "|") {
		t.Fatal("the mode off changed the rules")
	}
}

// The whole rule list: the local networks are refused before the Russian exception can
// let anything out, the exception comes before WARP's rules, MATCH is last.
func TestRUDirectRuleOrder(t *testing.T) {
	st := ruState(t)
	geoReady = func() bool { return true }
	defer func() { geoReady = func() bool { return geoOK.Load() } }()
	r := rules(st, false)
	pos := func(prefix string) int {
		for i, x := range r {
			if strings.HasPrefix(x, prefix) {
				return i
			}
		}
		return -1
	}
	priv, ru, exit, match := pos("IP-CIDR,10.0.0.0/8,REJECT"), pos("AND,((IN-NAME,in-anytls),(GEOSITE"), pos("IN-NAME,in-anytls,NODE-3"), pos("MATCH,")
	if priv < 0 || ru < 0 || exit < 0 || !(priv < ru && ru < exit && exit < match) || match != len(r)-1 {
		t.Fatalf("order private=%d ru=%d exit=%d match=%d of %d:\n%s", priv, ru, exit, match, len(r), strings.Join(r, "\n"))
	}
}

// The core takes the finished config: the rules parse with the databases it reads, and
// the exception comes out before the exit's rule when a Russian address is asked for.
func TestRUDirectConfigParses(t *testing.T) {
	e := geoEngine(t)
	if err := e.PutGeodata(GeoIP, testGeoIP(t, "ru")); err != nil {
		t.Fatal(err)
	}
	if err := e.PutGeodata(GeoSite, testGeoSite(t, "category-ru")); err != nil {
		t.Fatal(err)
	}
	C.SetHomeDir(e.home)
	st := ruState(t)
	raw, _, err := buildConfig(st, mproto.Cert{CertPath: "/tmp/c.pem", KeyPath: "/tmp/k.pem"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := executor.ParseWithBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	sawRU := 0
	for _, r := range cfg.Rules {
		if r.RuleType() == C.AND && strings.Contains(strings.ToUpper(r.Payload()), "GEO") && r.Adapter() == "DIRECT" {
			sawRU++
		}
	}
	if sawRU != 2 {
		t.Fatalf("%d Russian exceptions reached the core's rules, want the site's and the address's", sawRU)
	}
}
