//go:build windows

package build

import (
	"os/exec"
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

// TimeoutKillAction names, for the timeout report, the kill a timed-out
// recipe gets on this platform.
const TimeoutKillAction = "sent CTRL_BREAK to process group and terminated its job object, if any"

// windowsCreateNewProcessGroup is CREATE_NEW_PROCESS_GROUP. Defined
// locally so the file does not depend on x/sys/windows.
const windowsCreateNewProcessGroup = 0x00000200

// jobKiller is the groupKiller on Windows. job is the Job Object
// afterStart assigned the recipe to, or 0 when it could not set one up;
// then kill sends CTRL_BREAK alone.
type jobKiller struct {
	cmd *exec.Cmd
	job syscall.Handle
}

// afterStart assigns the started process to a freshly created Job Object
// configured to kill the whole tree when the handle closes. The killer
// it returns holds the handle; close closes it (and so kills any
// survivors) when the recipe completes. On any failure setting up the
// job the killer holds no job: the CREATE_NEW_PROCESS_GROUP flag still
// allows the CTRL_BREAK kill path.
func afterStart(cmd *exec.Cmd) groupKiller {
	k := &jobKiller{cmd: cmd}
	if cmd.Process == nil {
		return k
	}
	job, err := createKillOnCloseJob()
	if err != nil {
		return k
	}
	ph, err := openProcessForJob(cmd.Process.Pid)
	if err != nil {
		_ = closeHandle(job)
		return k
	}
	if err := assignProcessToJob(job, ph); err != nil {
		_ = closeHandle(ph)
		_ = closeHandle(job)
		return k
	}
	_ = closeHandle(ph)
	k.job = job
	return k
}

// kill sends CTRL_BREAK_EVENT to the recipe's process group, then
// terminates the Job Object so any survivors (including grandchildren)
// are killed. The job termination is the guaranteed kill path; the
// CTRL_BREAK is the polite first signal.
func (k *jobKiller) kill() {
	if k.cmd.Process == nil {
		return
	}
	sendCtrlBreak(k.cmd.Process.Pid)
	if k.job != 0 {
		_ = terminateJob(k.job)
	}
}

// forceLeader kills only the leader with TerminateProcess, which it
// cannot refuse. It covers a recipe that ignored CTRL_BREAK when no Job
// Object could be set up. A nil Process is a no-op.
func (k *jobKiller) forceLeader() { killCmdLeader(k.cmd) }

// close closes the job handle; KILL_ON_JOB_CLOSE reaps any survivors.
func (k *jobKiller) close() {
	if k.job != 0 {
		_ = closeHandle(k.job)
		k.job = 0
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
