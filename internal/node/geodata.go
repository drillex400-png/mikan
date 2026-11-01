package node

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/metacubex/mihomo/component/geodata"

	"mikan/internal/fsutil"
)

// Geodata for the "Russia direct" mode of a cascade (Exit.RUDirect). The rules GEOSITE and
// GEOIP need the databases when the config is parsed. A GEOIP rule whose database is
// missing is no soft error for the core: it fetches the file itself, and where that fails
// the node is down. So the node never lets a rule name a database it has not read: the
// panel hands it the files (PutGeodata), the node reads each one before it keeps it, and
// the rules are written only when both are in place. Without them the mode is off and the
// inbound goes to its exit whole, as it did before the mode.

// The databases, as v2ray's geoip.dat and geosite.dat (what 3X-UI uses), and the entry of
// each that the mode asks for: a file that parses but lacks it would match nothing.
const (
	GeoIP   = "geoip"
	GeoSite = "geosite"

	geoIPFile   = "GeoIP.dat"
	geoSiteFile = "GeoSite.dat"

	geoIPEntry   = "ru"
	geoSiteEntry = "category-ru"

	// MaxGeodata bounds a database a node accepts: geosite.dat is some 5 MB, geoip.dat 2.
	MaxGeodata = 32 << 20
)

var (
	// ErrGeodataName is a database that is no GeoIP and no GeoSite.
	ErrGeodataName = errors.New("geodata: unknown database")
	// ErrGeodataBad is a file that is not the database it was sent as.
	ErrGeodataBad = errors.New("geodata: not a usable database")

	geoMu sync.Mutex
	// geoOK is whether both databases are on disk and were read once; geoReady is what
	// the rules ask. Tests swap geoReady.
	geoOK    atomic.Bool
	geoReady = func() bool { return geoOK.Load() }
)

func geoFile(name string) (string, error) {
	switch name {
	case GeoIP:
		return geoIPFile, nil
	case GeoSite:
		return geoSiteFile, nil
	}
	return "", ErrGeodataName
}

// checkGeo reads data as the database name and says whether it holds the entry the mode
// needs. It touches no file and no global state of the core.
func checkGeo(name string, data []byte) error { return CheckGeodata(name, data) }

// CheckGeodata is what the node asks of a database before it keeps it; the panel asks the
// same of one it fetched, so it never sends what the node would refuse.
func CheckGeodata(name string, data []byte) error {
	if len(data) == 0 || len(data) > MaxGeodata {
		return ErrGeodataBad
	}
	loader, err := geodata.GetGeoDataLoader("standard")
	if err != nil {
		return err
	}
	switch name {
	case GeoIP:
		cidrs, err := loader.LoadIPByBytes(data, geoIPEntry)
		if err != nil || len(cidrs) == 0 {
			return fmt.Errorf("%w: no %q in geoip", ErrGeodataBad, geoIPEntry)
		}
	case GeoSite:
		sites, err := loader.LoadSiteByBytes(data, geoSiteEntry)
		if err != nil || len(sites) == 0 {
			return fmt.Errorf("%w: no %q in geosite", ErrGeodataBad, geoSiteEntry)
		}
	default:
		return ErrGeodataName
	}
	return nil
}

// PutGeodata keeps a database the panel sent: it is read first, written whole or not at
// all, and the mode is ready once both are in place. A file that does not read leaves the
// old one where it was.
func (e *Engine) PutGeodata(name string, data []byte) error {
	file, err := geoFile(name)
	if err != nil {
		return err
	}
	if err := checkGeo(name, data); err != nil {
		return err
	}
	geoMu.Lock()
	defer geoMu.Unlock()
	if err := fsutil.WriteFileAtomic(filepath.Join(e.home, file), data, 0o644); err != nil {
		return err
	}
	geoOK.Store(haveGeoFiles(e.home))
	return nil
}

// GeodataStatus says which databases the node holds.
func (e *Engine) GeodataStatus() (ip, site bool) {
	return fileSized(filepath.Join(e.home, geoIPFile)), fileSized(filepath.Join(e.home, geoSiteFile))
}

// loadGeo is the node's start: databases left on disk by an earlier run are read again
// before the mode counts on them.
func (e *Engine) loadGeo() {
	geoMu.Lock()
	defer geoMu.Unlock()
	for name, file := range map[string]string{GeoIP: geoIPFile, GeoSite: geoSiteFile} {
		data, err := os.ReadFile(filepath.Join(e.home, file))
		if err != nil || checkGeo(name, data) != nil {
			geoOK.Store(false)
			return
		}
	}
	geoOK.Store(true)
}

func fileSized(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}

// haveGeoFiles says whether both files are there and not empty.
func haveGeoFiles(home string) bool {
	return fileSized(filepath.Join(home, geoIPFile)) && fileSized(filepath.Join(home, geoSiteFile))
}
