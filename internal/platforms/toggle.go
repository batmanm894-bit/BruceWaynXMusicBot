package platforms

import (
	"sort"
	"strings"
	"sync"

	"github.com/Laky-64/gologging"

	state "main/internal/core/models"
	"main/internal/database"
)

// Owner-controlled on/off switch per platform. A platform that is "off" is
// never tried at all (no download attempt, no race slot, no error spam).
// The list lives in memory for speed and is saved in MongoDB so it survives
// restarts.

var disabledSet = struct {
	mu sync.RWMutex
	m  map[string]bool
}{m: map[string]bool{}}

func nameKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer("-", "", "_", "", " ", "").Replace(s)
}

// IsDisabled reports whether the owner switched this platform off.
func IsDisabled(n state.PlatformName) bool {
	disabledSet.mu.RLock()
	defer disabledSet.mu.RUnlock()
	return disabledSet.m[nameKey(string(n))]
}

// LoadDisabledFromDB restores the off-list at startup (call after the
// database is connected).
func LoadDisabledFromDB() {
	names, err := database.DisabledPlatforms()
	if err != nil {
		gologging.ErrorF("platform toggle: failed to load off-list: %v", err)
		return
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[nameKey(n)] = true
	}
	disabledSet.mu.Lock()
	disabledSet.m = m
	disabledSet.mu.Unlock()
	if len(m) > 0 {
		gologging.InfoF("Platforms switched off by owner: %s", strings.Join(names, ", "))
	}
}

func snapshotList() []string {
	list := make([]string, 0, len(disabledSet.m))
	for k := range disabledSet.m {
		list = append(list, k)
	}
	sort.Strings(list)
	return list
}

// SetDisabled switches a platform off (off=true) or on (off=false) and
// saves it.
func SetDisabled(n state.PlatformName, off bool) error {
	disabledSet.mu.Lock()
	if off {
		disabledSet.m[nameKey(string(n))] = true
	} else {
		delete(disabledSet.m, nameKey(string(n)))
	}
	list := snapshotList()
	disabledSet.mu.Unlock()
	return database.SetDisabledPlatforms(list)
}

// EnableAll turns every platform back on.
func EnableAll() error {
	disabledSet.mu.Lock()
	disabledSet.m = map[string]bool{}
	disabledSet.mu.Unlock()
	return database.SetDisabledPlatforms([]string{})
}

// RegisteredNames lists every registered platform, highest priority first.
func RegisteredNames() []state.PlatformName {
	ps := GetOrderedPlatforms()
	out := make([]state.PlatformName, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name())
	}
	return out
}

// ResolvePlatformName maps what the owner typed ("shruti", "yt-dlp",
// "fallen", "saavn") to a registered platform name.
func ResolvePlatformName(input string) (state.PlatformName, bool) {
	in := nameKey(input)
	aliases := map[string]string{
		"shruti":   "shrutiapi",
		"fallen":   "fallenapi",
		"jiosaavn": "saavn",
		"direct":   "directstream",
		"tg":       "telegram",
	}
	if a, ok := aliases[in]; ok {
		in = a
	}
	for _, n := range RegisteredNames() {
		if nameKey(string(n)) == in {
			return n, true
		}
	}
	return "", false
}
