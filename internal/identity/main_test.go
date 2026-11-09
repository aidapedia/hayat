package identity_test

import (
	"os"
	"testing"

	"github.com/aidapedia/hayat/internal/testdb"
)

// TestMain stops the package's embedded PostgreSQL when its tests finish.
func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }
