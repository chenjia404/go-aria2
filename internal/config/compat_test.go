package config

import "testing"

func TestApplyAria2CompatMode(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Aria2CompatMode = true
	ApplyAria2CompatMode(cfg)
	if !cfg.RPCStrictAuth {
		t.Fatal("expected rpc-strict-auth enabled in compat mode")
	}
}

func TestAria2SessionExportPath(t *testing.T) {
	t.Parallel()

	if got := Aria2SessionExportPath("./data/session.json"); got != "./data/session" {
		t.Fatalf("unexpected export path: %q", got)
	}
	if got := Aria2SessionExportPath("./data/session"); got != "./data/session.aria2" {
		t.Fatalf("unexpected export path: %q", got)
	}
}
