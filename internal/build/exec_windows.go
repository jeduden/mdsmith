//go:build windows

package build

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

// configureProcessGroup creates the recipe in a new process group so a
// CTRL_BREAK_EVENT can target the group on timeout. The Job Object that
// guarantees the kill path is created after Start (afterStart).
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windowsCreateNewProcessGroup,
	}
}

// windowsCreateNewProcessGroup is CREATE_NEW_PROCESS_GROUP. Defined
// locally so the file does not depend on x/sys/windows.
const windowsCreateNewProcessGroup = 0x00000200

// afterStart assigns the started process to a freshly created Job Object
// configured to kill the whole tree when the handle closes. It returns a
// cleanup closure that closes the job handle (and so kills any survivors)
// when the recipe completes. On any failure setting up the job it returns
// a no-op cleanup: the CREATE_NEW_PROCESS_GROUP flag still allows the
// CTRL_BREAK kill path.
func afterStart(cmd *exec.Cmd) func() {
	if cmd.Process == nil {
		return nil
	}
	job, err := createKillOnCloseJob()
	if err != nil {
		return nil
	}
	ph, err := openProcessForJob(cmd.Process.Pid)
	if err != nil {
		_ = closeHandle(job)
		return nil
	}
	if err := assignProcessToJob(job, ph); err != nil {
		_ = closeHandle(ph)
		_ = closeHandle(job)
		return nil
	}
	_ = closeHandle(ph)
	jobHandlesMu.Lock()
	jobHandles[cmd] = job
	jobHandlesMu.Unlock()
	return func() {
		_ = closeHandle(job) // KILL_ON_JOB_CLOSE reaps any survivors
		jobHandlesMu.Lock()
		delete(jobHandles, cmd)
		jobHandlesMu.Unlock()
	}
}

// jobHandles maps a running command to its Job Object handle so
// killGroup can terminate the whole job synchronously on timeout.
// jobHandlesMu guards it: Build is an exported method with no
// single-threaded contract, so afterStart, killGroup, and the cleanup
// closure may run from different goroutines if a caller dispatches
// recipes concurrently.
var (
	jobHandlesMu sync.Mutex
	jobHandles   = map[*exec.Cmd]syscall.Handle{}
)

// killGroup sends CTRL_BREAK_EVENT to the recipe's process group, then
// terminates the Job Object so any survivors (including grandchildren)
// are killed. The job termination is the guaranteed kill path; the
// CTRL_BREAK is the polite first signal.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	sendCtrlBreak(cmd.Process.Pid)
	jobHandlesMu.Lock()
	job, ok := jobHandles[cmd]
	jobHandlesMu.Unlock()
	if ok {
		_ = terminateJob(job)
	}
}

// --- thin syscall wrappers over kernel32 ---

var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObject          = modkernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = modkernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = modkernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = modkernel32.NewProc("TerminateJobObject")
	procOpenProcess              = modkernel32.NewProc("OpenProcess")
	procGenConsoleCtrlEvent      = modkernel32.NewProc("GenerateConsoleCtrlEvent")
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x00002000
	processAllAccess                  = 0x1F0FFF
	ctrlBreakEvent                    = 1
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

// jobObjectExtendedLimitInformationStruct mirrors the Win32
// JOBOBJECT_EXTENDED_LIMIT_INFORMATION layout. Only
// BasicLimitInformation.LimitFlags is written, but every field below is
// size- and offset-significant: SetInformationJobObject is passed
// unsafe.Sizeof(info), so removing a field (or the IoInfo counters) would
// shrink the struct and make the call fail, leaking survivor processes.
// Do not prune the unused fields.
type jobObjectExtendedLimitInformationStruct struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func createKillOnCloseJob() (syscall.Handle, error) {
	r, _, e := procCreateJobObject.Call(0, 0)
	if r == 0 {
		return 0, e
	}
	job := syscall.Handle(r)
	info := jobObjectExtendedLimitInformationStruct{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	rr, _, ee := procSetInformationJobObject.Call(
		uintptr(job),
		uintptr(jobObjectExtendedLimitInformation),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if rr == 0 {
		_ = closeHandle(job)
		return 0, ee
	}
	return job, nil
}

func openProcessForJob(pid int) (syscall.Handle, error) {
	r, _, e := procOpenProcess.Call(uintptr(processAllAccess), 0, uintptr(pid))
	if r == 0 {
		return 0, e
	}
	return syscall.Handle(r), nil
}

func assignProcessToJob(job, proc syscall.Handle) error {
	r, _, e := procAssignProcessToJobObject.Call(uintptr(job), uintptr(proc))
	if r == 0 {
		return e
	}
	return nil
}

func terminateJob(job syscall.Handle) error {
	r, _, e := procTerminateJobObject.Call(uintptr(job), 1)
	if r == 0 {
		return e
	}
	return nil
}

func sendCtrlBreak(pid int) {
	_, _, _ = procGenConsoleCtrlEvent.Call(uintptr(ctrlBreakEvent), uintptr(pid))
}

func closeHandle(h syscall.Handle) error {
	return syscall.CloseHandle(h)
}
