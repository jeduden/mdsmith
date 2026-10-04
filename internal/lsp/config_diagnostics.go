package lsp

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
)

// configLoadDiagnostic returns the diagnostic for a config load failure
// that carries a position in a config file, or nil when err has none.
func configLoadDiagnostic(err error) *lint.Diagnostic {
	var le *config.LoadError
	if !errors.As(err, &le) || !le.Positioned() || le.File == "" {
		return nil
	}
	d := le.Diagnostic()
	return &d
}

// publishConfigDiagnostic shows d as a squiggle on the config file it
// names, and clears the squiggle a previous reload left on a config
// file that no longer has one (d nil, or d on a different file). The
// column is measured against the open buffer when the file is open in
// the editor, else against the bytes on disk.
func (s *Server) publishConfigDiagnostic(d *lint.Diagnostic) {
	s.configDiagMu.Lock()
	defer s.configDiagMu.Unlock()

	uri := ""
	if d != nil {
		uri = pathToURI(d.File)
	}
	if prev := s.configDiagURI; prev != "" && prev != uri {
		_ = s.t.writeNotification("textDocument/publishDiagnostics",
			publishDiagnosticsParams{URI: prev, Diagnostics: []Diagnostic{}})
	}
	s.configDiagURI = uri
	if d == nil {
		return
	}

	var source []byte
	if doc, ok := s.docs.get(uri); ok {
		source = doc.text
	} else {
		// Missing bytes only cost the squiggle its columns: it then
		// spans the start of the reported line.
		source, _ = symbolWorkspace.ReadFile(d.File)
	}
	s.configMu.RLock()
	root := s.rootDir
	s.configMu.RUnlock()
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Diagnostics: toLSPAll([]lint.Diagnostic{*d}, source, root)})
}

// discoverWithHints is the production discoverConfig: one walk from
// root that returns the config file it finds and the hints it collects.
// The walk itself cannot fail; the error return serves test seams.
func discoverWithHints(root string) (string, []string, error) {
	found, hints := config.DiscoverWithHints(root)
	return found, hints, nil
}

// logDiscoverHints reports, as warnings, the hints a config discovery
// walk collected — a pyproject.toml with a plural `[tools.mdsmith]`
// table that is not read as config. Hints identical to the ones the
// previous reload logged are not repeated, so a settings change or an
// unrelated config event does not re-warn.
func (s *Server) logDiscoverHints(hints []string) {
	key := strings.Join(hints, "\n")
	s.hintsMu.Lock()
	changed := key != s.loggedHints
	s.loggedHints = key
	s.hintsMu.Unlock()
	if !changed {
		return
	}
	for _, hint := range hints {
		s.logger.Printf("config: %s", hint)
		_ = s.t.writeNotification("window/logMessage",
			logMessageParams{Type: messageTypeWarning, Message: "mdsmith: " + hint})
	}
}

// isWatchedConfigChange reports whether a watched file event on path
// must reload config. Only a config-named file (config.IsConfigFile)
// qualifies, and then only one that can change what reloadConfig
// loads: the loaded config file itself; with an `mdsmith.config`
// override, the override file; otherwise a file in the workspace root
// or one of its ancestors, the directories discovery walks. A
// pyproject.toml nested below the root — every package of a Python
// monorepo has one — is never read, so editing it must not rebuild
// the session. With no root known yet, every config-named file counts.
func (s *Server) isWatchedConfigChange(path string) bool {
	if !config.IsConfigFile(path) {
		return false
	}
	path = filepath.Clean(path)
	s.settingsMu.RLock()
	override := s.settings.ConfigPath
	s.settingsMu.RUnlock()
	s.configMu.RLock()
	loaded, root := s.configPath, s.rootDir
	s.configMu.RUnlock()
	if loaded != "" && path == filepath.Clean(loaded) {
		return true
	}
	if override != "" {
		if !filepath.IsAbs(override) && root != "" {
			override = filepath.Join(root, override)
		}
		return path == filepath.Clean(override)
	}
	if root == "" {
		return true
	}
	return isDirOrAncestor(filepath.Dir(path), filepath.Clean(root))
}

// isDirOrAncestor reports whether dir is root or one of its ancestors.
func isDirOrAncestor(dir, root string) bool {
	rel, err := filepath.Rel(dir, root)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
