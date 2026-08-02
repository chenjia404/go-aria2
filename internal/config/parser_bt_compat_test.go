package config

import (
	"strings"
	"testing"
)

func TestParseBTCompatOptions(t *testing.T) {
	t.Parallel()

	cfg, err := Parse(strings.NewReader(`check-integrity=true
bt-enable-lpd=true
bt-detach-seed-only=true
bt-remove-unselected-file=true
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.CheckIntegrity || !cfg.BTEnableLPD || !cfg.BTDetachSeedOnly || !cfg.BTRemoveUnselectedFile {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
