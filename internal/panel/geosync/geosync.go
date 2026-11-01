// Package geosync keeps the geodata the "Russia direct" cascade mode needs and hands it to
// the nodes that use the mode: geoip.dat and geosite.dat, as v2ray reads them (what 3X-UI
// uses). The panel fetches them, reads each before keeping it, holds the last good pair on
// disk and sends a node its copy when the node has none or an older one. A node never goes
// to the internet for them: it is not trusted to, and a database it fetched itself and
// could not read would stop it.
package geosync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mikan/internal/fsutil"
	"mikan/internal/node"
)

// The databases, by the names the Node API knows them.
const (
	GeoIP   = node.GeoIP
	GeoSite = node.GeoSite
)

// Source is where one database comes from. geoip.dat must hold Russia ("ru"): the geoip-lite
// of MetaCubeX does not, so it comes from the build that adds the country lists; the geosite
// of MetaCubeX holds category-ru and is a fraction of the size.
type Source struct {
	Name string
	URL  string
}

// DefaultSources are the databases' places, tried in order for each.
var DefaultSources = map[string][]Source{
	GeoIP: {
		{"runetfreedom", "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat"},
		{"runetfreedom-mirror", "https://cdn.jsdelivr.net/gh/runetfreedom/russia-v2ray-rules-dat@release/geoip.dat"},
	},
	GeoSite: {
		{"metacubex", "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"},
		{"runetfreedom", "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat"},
	},
}

const (
	// Every is how often the databases are fetched again.
	Every = 3 * 24 * time.Hour
	// retry is the wait after a fetch that failed, so a down source is not hammered.
	retry = 30 * time.Minute
	// fetchLimit bounds one download; a node takes node.MaxGeodata.
	fetchLimit = node.MaxGeodata
)

var (
	// ErrNone: no database is held yet.
	ErrNone = errors.New("geosync: no geodata yet")
	// ErrUnusable: a fetched file is not the database it should be.
	ErrUnusable = errors.New("geosync: not a usable database")
)

// Set is the pair held: the files' bytes and their digests.
type Set struct {
	IP, Site       []byte
	IPSum, SiteSum string
	FetchedAt      time.Time
}

// Service holds the databases and sends them.
type Service struct {
	dir     string
	hc      *http.Client
	sources map[string][]Source
	log     *slog.Logger
	now     func() time.Time

	mu      sync.Mutex
	set     *Set
	sent    map[int64]string // node -> "ipsum+sitesum" it holds
	lastErr string
	nextTry time.Time
}

// New keeps its files under dir/geodata.
func New(dataDir string, log *slog.Logger, now func() time.Time) *Service {
	return &Service{
		dir: filepath.Join(dataDir, "geodata"), hc: &http.Client{Timeout: 3 * time.Minute},
		sources: DefaultSources, log: log, now: now, sent: map[int64]string{},
	}
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Load takes the pair left on disk by an earlier run, when both files still read.
func (s *Service) Load() error {
	ip, err1 := os.ReadFile(filepath.Join(s.dir, "geoip.dat"))
	site, err2 := os.ReadFile(filepath.Join(s.dir, "geosite.dat"))
	if err := errors.Join(err1, err2); err != nil {
		return ErrNone
	}
	if node.CheckGeodata(GeoIP, ip) != nil || node.CheckGeodata(GeoSite, site) != nil {
		return ErrUnusable
	}
	var at time.Time
	if fi, err := os.Stat(filepath.Join(s.dir, "geoip.dat")); err == nil {
		at = fi.ModTime()
	}
	s.mu.Lock()
	s.set = &Set{IP: ip, Site: site, IPSum: sum(ip), SiteSum: sum(site), FetchedAt: at}
	s.mu.Unlock()
	return nil
}

// fetch takes one database from the first source that gives a usable file.
func (s *Service) fetch(ctx context.Context, name string) ([]byte, error) {
	var errs []error
	for _, src := range s.sources[name] {
		b, err := s.get(ctx, src.URL)
		if err == nil {
			err = node.CheckGeodata(name, b)
		}
		if err == nil {
			return b, nil
		}
		s.log.Warn("geodata source", "db", name, "source", src.Name, "err", err)
		errs = append(errs, fmt.Errorf("%s: %w", src.Name, err))
	}
	return nil, errors.Join(errs...)
}

func (s *Service) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, fetchLimit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > fetchLimit {
		return nil, ErrUnusable
	}
	return b, nil
}

// Refresh fetches both databases and keeps them when both read; otherwise the pair held
// stays as it was.
func (s *Service) Refresh(ctx context.Context) error {
	ip, err := s.fetch(ctx, GeoIP)
	if err != nil {
		return s.failed(err)
	}
	site, err := s.fetch(ctx, GeoSite)
	if err != nil {
		return s.failed(err)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return s.failed(err)
	}
	for f, b := range map[string][]byte{"geoip.dat": ip, "geosite.dat": site} {
		if err := fsutil.WriteFileAtomic(filepath.Join(s.dir, f), b, 0o644); err != nil {
			return s.failed(err)
		}
	}
	s.mu.Lock()
	s.set = &Set{IP: ip, Site: site, IPSum: sum(ip), SiteSum: sum(site), FetchedAt: s.now()}
	s.lastErr = ""
	s.nextTry = s.now().Add(Every)
	s.mu.Unlock()
	s.log.Info("geodata refreshed", "geoip", len(ip), "geosite", len(site))
	return nil
}

func (s *Service) failed(err error) error {
	s.mu.Lock()
	s.lastErr = err.Error()
	s.nextTry = s.now().Add(retry)
	s.mu.Unlock()
	return err
}

// Held is the pair kept, nil when there is none.
func (s *Service) Held() *Set {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set
}

// Status is what the admin sees: whether a pair is held, how old, the last failure.
type Status struct {
	Have      bool      `json:"have"`
	FetchedAt time.Time `json:"fetched_at,omitzero"`
	Error     string    `json:"error,omitempty"`
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Have: s.set != nil, FetchedAt: s.set.fetchedAt(), Error: s.lastErr}
}

func (s *Set) fetchedAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.FetchedAt
}

// Ensure gives node id the pair held, once per pair: what it holds is remembered, and a
// node that restarted or lost its files is sent them again by Forget. With no pair held it
// fetches one. A nil error means the node has both databases.
func (s *Service) Ensure(ctx context.Context, p interface {
	PutGeodata(ctx context.Context, id int64, name string, data []byte) error
}, id int64) error {
	if s.Held() == nil {
		if err := s.Refresh(ctx); err != nil {
			return err
		}
	}
	set := s.Held()
	if set == nil {
		return ErrNone
	}
	want := set.IPSum + set.SiteSum
	s.mu.Lock()
	have := s.sent[id]
	s.mu.Unlock()
	if have == want {
		return nil
	}
	if err := p.PutGeodata(ctx, id, GeoIP, set.IP); err != nil {
		return err
	}
	if err := p.PutGeodata(ctx, id, GeoSite, set.Site); err != nil {
		return err
	}
	s.mu.Lock()
	s.sent[id] = want
	s.mu.Unlock()
	return nil
}

// Forget drops what is remembered of node id's databases: its next Ensure sends them again.
func (s *Service) Forget(id int64) {
	s.mu.Lock()
	delete(s.sent, id)
	s.mu.Unlock()
}

// Run keeps the databases fresh until ctx ends: it takes the pair on disk, fetches when
// there is none or it is older than Every, and waits.
func (s *Service) Run(ctx context.Context) {
	if err := s.Load(); err != nil && !errors.Is(err, ErrNone) {
		s.log.Warn("geodata on disk", "err", err)
	}
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		s.mu.Lock()
		stale := s.set == nil || s.now().Sub(s.set.FetchedAt) >= Every
		wait := s.now().Before(s.nextTry)
		s.mu.Unlock()
		// A fetch is only made while some node asks for the mode (Ensure fetches then); a
		// pair already held is kept fresh.
		if !wait && stale && s.Held() != nil {
			if err := s.Refresh(ctx); err != nil {
				s.log.Warn("geodata refresh", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
