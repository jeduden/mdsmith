package mdsmith

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
)

// TestConfigCompiledUsedAsIs verifies a ConfigCompiled source is taken
// exactly as supplied: NewSession does not re-merge it over defaults
// (which would drop settings a caller injected onto the compiled config,
// e.g. build recipes for MDS040) and reports the supplied config path.
// The CLI uses this so its loadConfig side effects (InjectBuildConfig,
// the include-extract projector) survive into the session.
func TestConfigCompiledUsedAsIs(t *testing.T) {
	// Build a config the way the CLI does, then mark a sentinel on a
	// rule's settings the way InjectBuildConfig would. If NewSession
	// re-merged over defaults, the sentinel-bearing entry would be
	// recomputed and the marker lost.
	merged := config.Merge(config.Defaults(), nil)
	if merged.Rules == nil {
		merged.Rules = map[string]config.RuleCfg{}
	}
	rc := merged.Rules["line-length"]
	if rc.Settings == nil {
		rc.Settings = map[string]any{}
	}
	rc.Settings["max"] = 999
	merged.Rules["line-length"] = rc

	src := ConfigCompiled(merged, "/proj/.mdsmith.yml")
	if got := src.configPath(); got != "/proj/.mdsmith.yml" {
		t.Fatalf("ConfigCompiled.configPath() = %q, want /proj/.mdsmith.yml", got)
	}

	s, err := NewSession(SessionOptions{Workspace: NewMemWorkspace(nil), Config: src})
	if err != nil {
		t.Fatalf("NewSession(ConfigCompiled): %v", err)
	}
	defer s.Dispose()

	if got := s.cfg.Rules["line-length"].Settings["max"]; got != 999 {
		t.Fatalf("session config line-length.max = %v, want 999 "+
			"(compiled config must be used as-is, not re-merged)", got)
	}
	if s.cfg != merged {
		t.Fatalf("session should hold the supplied compiled config pointer unchanged")
	}
	if s.cfgPath != "/proj/.mdsmith.yml" {
		t.Fatalf("session cfgPath = %q, want /proj/.mdsmith.yml", s.cfgPath)
	}
	if s.rootDir != "" {
		t.Fatalf("session rootDir = %q, want empty (MemWorkspace has no Root)", s.rootDir)
	}
}
