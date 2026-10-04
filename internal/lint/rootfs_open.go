package lint

// openRootFSFunc is the opener OpenRootFS calls: openRootFS in
// production. WrapOpenRootFS replaces it so one test hook, shared by
// every package that opens a root, can check each root is closed.
var openRootFSFunc = openRootFS

// OpenRootFS returns a RootFS rooted at dir (see openRootFS for the
// containment it enforces). The caller owns the returned handle and
// closes it once its reads end.
func OpenRootFS(dir string) RootFS {
	return openRootFSFunc(dir)
}

// WrapOpenRootFS makes OpenRootFS call wrap(current), where current is
// the opener it calls now, and returns a func that restores current.
// It is a test hook (see rootfstest.Record): a test that wraps it must
// not run in parallel with others that open roots.
func WrapOpenRootFS(wrap func(current func(string) RootFS) func(string) RootFS) (restore func()) {
	current := openRootFSFunc
	openRootFSFunc = wrap(current)
	return func() { openRootFSFunc = current }
}
