package geosync

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/geodata/router"
	"google.golang.org/protobuf/proto"
)

func geoIP(t *testing.T, country string) []byte {
	t.Helper()
	ip := netip.MustParseAddr("77.88.8.8").As4()
	b, err := proto.Marshal(&router.GeoIPList{Entry: []*router.GeoIP{{CountryCode: country, Cidr: []*router.CIDR{{Ip: ip[:], Prefix: 24}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func geoSite(t *testing.T, list string) []byte {
	t.Helper()
	b, err := proto.Marshal(&router.GeoSiteList{Entry: []*router.GeoSite{{CountryCode: list, Domain: []*router.Domain{{Type: router.Domain_Domain, Value: "yandex.ru"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type pusher struct {
	got  map[int64]map[string][]byte
	fail error
	n    int
}

func (p *pusher) PutGeodata(_ context.Context, id int64, name string, data []byte) error {
	p.n++
	if p.fail != nil {
		return p.fail
	}
	if p.got == nil {
		p.got = map[int64]map[string][]byte{}
	}
	if p.got[id] == nil {
		p.got[id] = map[string][]byte{}
	}
	p.got[id][name] = data
	return nil
}

// serve answers each path with the file given; a missing path is 404.
func serve(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newService(t *testing.T, srv *httptest.Server, ip, site []string) *Service {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	s := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now })
	s.hc = srv.Client()
	s.sources = map[string][]Source{}
	for _, p := range ip {
		s.sources[GeoIP] = append(s.sources[GeoIP], Source{Name: p, URL: srv.URL + p})
	}
	for _, p := range site {
		s.sources[GeoSite] = append(s.sources[GeoSite], Source{Name: p, URL: srv.URL + p})
	}
	return s
}

// A good pair is fetched, kept on disk and read back by the next run.
func TestRefreshKeepsGoodPair(t *testing.T) {
	srv := serve(t, map[string][]byte{"/ip": geoIP(t, "RU"), "/site": geoSite(t, "CATEGORY-RU")})
	s := newService(t, srv, []string{"/ip"}, []string{"/site"})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); !st.Have || st.Error != "" {
		t.Fatalf("status %+v", st)
	}
	for _, f := range []string{"geoip.dat", "geosite.dat"} {
		if _, err := os.Stat(filepath.Join(s.dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	again := New(filepath.Dir(s.dir), slog.New(slog.NewTextHandler(io.Discard, nil)), s.now)
	if err := again.Load(); err != nil || again.Held() == nil {
		t.Fatalf("a restart lost the pair: %v", err)
	}
}

// The first source gives a file that is not a database, or one without Russia: the next
// is used, and the bad one is never kept.
func TestRefreshFallsBack(t *testing.T) {
	srv := serve(t, map[string][]byte{
		"/bad-ip": []byte("<html>rate limited</html>"), "/ip": geoIP(t, "RU"),
		"/us-site": geoSite(t, "GEOLOCATION-!CN"), "/site": geoSite(t, "CATEGORY-RU"),
	})
	s := newService(t, srv, []string{"/bad-ip", "/ip"}, []string{"/us-site", "/site"})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.dir, "geoip.dat"))
	if string(b) != string(geoIP(t, "RU")) {
		t.Fatal("the bad source's file was kept")
	}
	b, _ = os.ReadFile(filepath.Join(s.dir, "geosite.dat"))
	if string(b) != string(geoSite(t, "CATEGORY-RU")) {
		t.Fatal("the geosite without Russia was kept")
	}
}

// When every source fails the pair already held stays, the failure is told, and the next
// try waits.
func TestRefreshFailureKeepsOldPair(t *testing.T) {
	good := serve(t, map[string][]byte{"/ip": geoIP(t, "RU"), "/site": geoSite(t, "CATEGORY-RU")})
	s := newService(t, good, []string{"/ip"}, []string{"/site"})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	held := s.Held()

	s.sources = map[string][]Source{GeoIP: {{Name: "gone", URL: good.URL + "/nothing"}}, GeoSite: s.sources[GeoSite]}
	if err := s.Refresh(context.Background()); err == nil {
		t.Fatal("a failed fetch reported success")
	}
	if s.Held() != held {
		t.Fatal("a failed fetch replaced the pair held")
	}
	if st := s.Status(); !st.Have || st.Error == "" {
		t.Fatalf("status %+v", st)
	}
	b, _ := os.ReadFile(filepath.Join(s.dir, "geoip.dat"))
	if string(b) != string(geoIP(t, "RU")) {
		t.Fatal("the file on disk was touched")
	}
}

// A file bigger than a node takes is refused.
func TestRefreshRefusesHuge(t *testing.T) {
	huge := make([]byte, fetchLimit+10)
	srv := serve(t, map[string][]byte{"/ip": huge, "/site": geoSite(t, "CATEGORY-RU")})
	s := newService(t, srv, []string{"/ip"}, []string{"/site"})
	if err := s.Refresh(context.Background()); err == nil || s.Held() != nil {
		t.Fatalf("a file over the limit was taken: %v", err)
	}
}

// Ensure sends a node the pair once, again after Forget; a node that refuses is not
// recorded as having it and is asked again.
func TestEnsure(t *testing.T) {
	srv := serve(t, map[string][]byte{"/ip": geoIP(t, "RU"), "/site": geoSite(t, "CATEGORY-RU")})
	s := newService(t, srv, []string{"/ip"}, []string{"/site"})
	ctx := context.Background()
	p := &pusher{}

	// Nothing held: Ensure fetches first.
	if err := s.Ensure(ctx, p, 7); err != nil {
		t.Fatal(err)
	}
	if p.n != 2 || len(p.got[7]) != 2 {
		t.Fatalf("pushed %d, node holds %v", p.n, p.got)
	}
	if err := s.Ensure(ctx, p, 7); err != nil || p.n != 2 {
		t.Fatalf("sent again with nothing new: n=%d err=%v", p.n, err)
	}
	s.Forget(7)
	if err := s.Ensure(ctx, p, 7); err != nil || p.n != 4 {
		t.Fatalf("not sent after Forget: n=%d err=%v", p.n, err)
	}
	// Another node has its own record.
	if err := s.Ensure(ctx, p, 8); err != nil || p.n != 6 {
		t.Fatalf("second node: n=%d err=%v", p.n, err)
	}

	boom := errors.New("refused")
	q := &pusher{fail: boom}
	if err := s.Ensure(ctx, q, 9); !errors.Is(err, boom) {
		t.Fatalf("refusal lost: %v", err)
	}
	q.fail = nil
	if err := s.Ensure(ctx, q, 9); err != nil || len(q.got[9]) != 2 {
		t.Fatalf("not retried after a refusal: %v %v", err, q.got)
	}
}

// With no source answering and nothing held, Ensure fails and sends nothing.
func TestEnsureNoDatabases(t *testing.T) {
	srv := serve(t, map[string][]byte{})
	s := newService(t, srv, []string{"/ip"}, []string{"/site"})
	p := &pusher{}
	if err := s.Ensure(context.Background(), p, 1); err == nil || p.n != 0 {
		t.Fatalf("err=%v pushed=%d", err, p.n)
	}
}

// Nothing on disk is no pair; a torn file is not trusted.
func TestLoadRejectsTornFiles(t *testing.T) {
	s := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	if err := s.Load(); !errors.Is(err, ErrNone) {
		t.Fatalf("empty dir: %v", err)
	}
	_ = os.MkdirAll(s.dir, 0o755)
	_ = os.WriteFile(filepath.Join(s.dir, "geoip.dat"), geoIP(t, "RU"), 0o644)
	_ = os.WriteFile(filepath.Join(s.dir, "geosite.dat"), []byte("torn"), 0o644)
	if err := s.Load(); !errors.Is(err, ErrUnusable) || s.Held() != nil {
		t.Fatalf("torn file: %v", err)
	}
}
