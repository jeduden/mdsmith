package build

import (
	"io"
	"os"
	"os/exec"
	"sync"
)

// recipeOutput carries a recipe's stdout and stderr to the caller's
// writers through pipes runRecipe owns, instead of letting os/exec
// create them. With os/exec's pipes, cmd.Wait cannot return while any
// process (say, a daemon that escaped the group kill) still holds a
// write end, and once runRecipe stopped waiting the Wait goroutine,
// the copy goroutines, and the read fds stayed alive as long as that
// survivor did. Here cmd.Wait returns when the leader exits, and
// abandon closes the read ends, which ends every copy goroutine.
//
// A writer that is an *os.File (including the nil default, os.Stderr)
// goes to the child directly, with no pipe, as os/exec does.
type recipeOutput struct {
	gate     outputGate
	readers  []*os.File // parent read ends, one per pipe
	children []*os.File // child write ends, closed after Start
	copies   sync.WaitGroup
	drained  chan struct{} // closed when every copy goroutine ended

	errMu sync.Mutex
	// copyErr is the first error a copy goroutine hit: a failed write
	// to a caller's writer, or a read error. After abandon it holds
	// the closed-file read error, so only read it on a run that
	// drained without abandon.
	copyErr error
}

// attach sets cmd.Stdout and cmd.Stderr, creating a pipe and starting
// a copy goroutine for each writer that is not an *os.File. A nil
// writer means os.Stderr. One writer passed for both streams shares a
// single pipe, so it never sees two concurrent Write calls. On error
// every pipe it created is closed.
func (ro *recipeOutput) attach(cmd *exec.Cmd, stdout, stderr io.Writer) error {
	ro.drained = make(chan struct{})
	if stdout == nil {
		stdout = os.Stderr
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	out, err := ro.route(stdout)
	if err == nil {
		cmd.Stdout, cmd.Stderr = out, out
		if !interfaceEqual(stdout, stderr) {
			cmd.Stderr, err = ro.route(stderr)
		}
	}
	go func() {
		ro.copies.Wait()
		close(ro.drained)
	}()
	if err != nil {
		ro.closeChildEnds()
		ro.abandon()
	}
	return err
}

// route returns what the child should write to for w: w itself when it
// is an *os.File, otherwise the write end of a new pipe whose read end
// a goroutine copies into w through the gate.
func (ro *recipeOutput) route(w io.Writer) (io.Writer, error) {
	if f, ok := w.(*os.File); ok {
		return f, nil
	}
	pr, pw, err := pipeFn()
	if err != nil {
		return nil, err
	}
	ro.readers = append(ro.readers, pr)
	ro.children = append(ro.children, pw)
	ro.copies.Add(1)
	go ro.copy(ro.gate.wrap(w), pr)
	return pw, nil
}

// pipeFn indirects os.Pipe so a test can make pipe creation fail.
var pipeFn = os.Pipe

// copy drains pr into w until EOF or until pr is closed. If w fails,
// it records the error and closes pr, so the child sees EPIPE rather
// than blocking on a full pipe (os/exec does the same).
func (ro *recipeOutput) copy(w io.Writer, pr *os.File) {
	defer ro.copies.Done()
	_, err := io.Copy(w, pr)
	_ = pr.Close()
	if err != nil {
		ro.errMu.Lock()
		if ro.copyErr == nil {
			ro.copyErr = err
		}
		ro.errMu.Unlock()
	}
}

// closeChildEnds closes the parent's copies of the child write ends.
// Call it once Start has returned: the child holds its own copies, and
// EOF on the read end needs every write end closed.
func (ro *recipeOutput) closeChildEnds() {
	for _, f := range ro.children {
		_ = f.Close()
	}
}

// err returns the first error a copy goroutine hit (see copyErr).
// Read it only after drained is closed, and never after abandon.
func (ro *recipeOutput) err() error {
	ro.errMu.Lock()
	defer ro.errMu.Unlock()
	return ro.copyErr
}

// abandon closes every read end. On Unix and Windows a copy goroutine
// blocked in Read returns, and a survivor still holding a write end
// gets EPIPE on its next write. plan9 cannot cancel a pending read, so
// there the goroutine and fd stay until the survivor's next write or
// exit, and that write succeeds. A copy goroutine that already read
// data may still pass it to the gate; the gate's close (on runRecipe's
// return) drops it.
func (ro *recipeOutput) abandon() {
	for _, f := range ro.readers {
		_ = f.Close()
	}
}

// interfaceEqual reports a == b, treating a comparison that panics
// (two values of the same non-comparable type) as unequal, as os/exec
// does when it checks Stdout == Stderr.
func interfaceEqual(a, b any) (eq bool) {
	defer func() {
		if recover() != nil {
			eq = false
		}
	}()
	return a == b
}

// outputGate forwards a recipe's stdout and stderr writes until close,
// then drops them. One mutex covers both streams and is held across
// each forwarded Write, so once close returns no write is in flight
// and none follows.
type outputGate struct {
	mu     sync.Mutex
	closed bool
}

// wrap returns a writer that forwards to w while the gate is open.
func (g *outputGate) wrap(w io.Writer) io.Writer { return gatedWriter{g: g, w: w} }

// close stops forwarding, after any in-flight write completes.
func (g *outputGate) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

// gatedWriter is one stream's side of an outputGate.
type gatedWriter struct {
	g *outputGate
	w io.Writer
}

// Write forwards p to the wrapped writer, or reports it written and
// drops it once the gate is closed, so a copy goroutine that already
// read data finishes quietly.
func (gw gatedWriter) Write(p []byte) (int, error) {
	gw.g.mu.Lock()
	defer gw.g.mu.Unlock()
	if gw.g.closed {
		return len(p), nil
	}
	return gw.w.Write(p)
}
