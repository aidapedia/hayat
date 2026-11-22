package apiserver

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every RPC of the service API says the release it first shipped in, so a
// service can work out the oldest Hayat it needs: the newest Since among the
// RPCs it calls. A new RPC says the release it will ship in.
func TestEveryServiceRPCSaysSince(t *testing.T) {
	src, err := os.ReadFile("../../proto/hayat/authz/v1/authz.proto")
	if err != nil {
		t.Fatal(err)
	}
	rpc := regexp.MustCompile(`^\s*rpc (\w+)\(`)
	since := regexp.MustCompile(`^\s*// Since \d+\.\d+\.\d+\.$`)
	lines := strings.Split(string(src), "\n")
	seen := 0
	for i, line := range lines {
		m := rpc.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		seen++
		ok := false
		// The comment block right above the rpc carries it.
		for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "//"); j-- {
			if since.MatchString(lines[j]) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf(`rpc %s (authz.proto:%d) needs a "// Since X.Y.Z." line above it: the Hayat release it ships in`, m[1], i+1)
		}
	}
	if seen == 0 {
		t.Fatal("found no rpc in authz.proto")
	}
}
