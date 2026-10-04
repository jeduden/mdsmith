package lsp

import (
	"errors"

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
		// Missing bytes only lose the squiggle's end column.
		source, _ = symbolWorkspace.ReadFile(d.File)
	}
	s.configMu.RLock()
	root := s.rootDir
	s.configMu.RUnlock()
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Diagnostics: toLSPAll([]lint.Diagnostic{*d}, source, root)})
}
