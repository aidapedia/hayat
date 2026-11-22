package hayat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"connectrpc.com/connect"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/authz/v1/authzv1connect"
)

// VersionFloor is the oldest Hayat any service can declare: the first release
// that reports its version (ServerInfoService). A lower MinHayatVersion is
// raised to it, since an older Hayat can't say what it is.
const VersionFloor = "1.2.0"

// ErrHayatTooOld is what errors.Is finds in a *VersionError.
var ErrHayatTooOld = errors.New("hayat: this Hayat is older than the service needs")

// VersionError says the Hayat a service reached is older than it declared it
// needs. Have is empty when that Hayat is too old to report its version.
type VersionError struct {
	Have, Need, APIURL string
}

func (e *VersionError) Error() string {
	have := e.Have
	if have == "" {
		have = "a release older than " + VersionFloor + " (it doesn't report its version)"
	}
	return fmt.Sprintf("hayat: this service needs Hayat %s or newer, but %s runs %s — upgrade Hayat first", e.Need, e.APIURL, have)
}

func (e *VersionError) Is(target error) bool { return target == ErrHayatTooOld }

// CheckVersion asks the Hayat at apiURL which release it is and refuses one
// older than min (raised to VersionFloor). It needs no credentials, so a
// service calls it before it enrolls; New calls it on every start. A build
// from source ("dev") is new enough. It returns the version Hayat reported.
func CheckVersion(ctx context.Context, apiURL, min string, hc *http.Client) (string, error) {
	need, err := minVersion(min)
	if err != nil {
		return "", err
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	api := strings.TrimRight(apiURL, "/")
	resp, err := authzv1connect.NewServerInfoServiceClient(hc, api).GetServerInfo(ctx, connect.NewRequest(&authzv1.GetServerInfoRequest{}))
	if err != nil {
		if c := connect.CodeOf(err); c == connect.CodeUnimplemented || c == connect.CodeNotFound {
			return "", &VersionError{Need: need, APIURL: api}
		}
		return "", fmt.Errorf("hayat: couldn't read Hayat's version at %s: %w", api, err)
	}
	have := resp.Msg.Version
	if !versionAtLeast(have, need) {
		return have, &VersionError{Have: have, Need: need, APIURL: api}
	}
	return have, nil
}

// minVersion validates a declared minimum and raises it to VersionFloor.
func minVersion(min string) (string, error) {
	if min == "" {
		return "", errors.New(`hayat: MinHayatVersion is required — the oldest Hayat release this service works with: ` +
			`the newest "Since" among the RPCs it calls (proto/hayat/authz/v1), e.g. "` + VersionFloor + `"`)
	}
	v, ok := parseVersion(min)
	if !ok {
		return "", fmt.Errorf("hayat: MinHayatVersion %q isn't a release like %q", min, VersionFloor)
	}
	floor, _ := parseVersion(VersionFloor)
	if compare(v, floor) < 0 {
		return VersionFloor, nil
	}
	return strings.TrimPrefix(min, "v"), nil
}

// versionAtLeast reports whether have >= need. A version that isn't a release
// ("dev", a build from source) is new enough: it is whatever the source is.
func versionAtLeast(have, need string) bool {
	h, ok := parseVersion(have)
	if !ok {
		return true
	}
	n, _ := parseVersion(need)
	return compare(h, n) >= 0
}

// parseVersion reads "1.2.3" (a leading v and a "-rc.1" or "+build" suffix
// are allowed; the suffix is ignored).
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func compare(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
