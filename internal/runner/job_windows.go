//go:build windows

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"golang.org/x/sys/windows"
)

// PROC_THREAD_ATTRIBUTE_JOB_LIST is supported on Windows 10 and later.
const procThreadAttributeJobList = 0x0002000d

func runPlatformProcess(ctx context.Context, command domain.CommandSpec, onEvent func(Event)) (Result, error, bool) {
	result, err := runWindowsProcess(ctx, command, onEvent)
	return result, err, true
}

// runWindowsProcess creates the process in the job before its first instruction.
// The job is deliberately not inheritable: losing its last handle terminates the tree.
func runWindowsProcess(ctx context.Context, command domain.CommandSpec, onEvent func(Event)) (Result, error) {
	return runWindowsProcessWithTerminate(ctx, command, onEvent, windows.TerminateJobObject)
}

func runWindowsProcessWithTerminate(ctx context.Context, command domain.CommandSpec, onEvent func(Event), terminate func(windows.Handle, uint32) error) (Result, error) {
	if command.Exe == "" {
		return Result{}, fmt.Errorf("empty executable")
	}
	if command.WorkingDir != "" {
		if _, err := os.Stat(command.WorkingDir); err != nil {
			return Result{}, fmt.Errorf("working directory %q: %w", command.WorkingDir, err)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	if command.TimeoutSeconds > 0 {
		cancel()
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(command.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	if err := runCtx.Err(); err != nil {
		return Result{}, err
	}

	lookup := command.Exe
	if filepath.Base(lookup) != lookup && !filepath.IsAbs(lookup) && command.WorkingDir != "" {
		lookup = filepath.Join(command.WorkingDir, lookup)
		var absErr error
		lookup, absErr = filepath.Abs(lookup)
		if absErr != nil {
			return Result{}, absErr
		}
	}
	path, err := exec.LookPath(lookup)
	if err != nil {
		return Result{}, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Result{}, err
	}
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Result{}, err
	}
	args := append([]string{command.Exe}, command.Args...)
	line, err := windows.UTF16FromString(windows.ComposeCommandLine(args))
	if err != nil {
		return Result{}, err
	}
	var dir *uint16
	if command.WorkingDir != "" {
		dir, err = windows.UTF16PtrFromString(command.WorkingDir)
		if err != nil {
			return Result{}, err
		}
	}
	env, err := windowsEnvironment(command.Env)
	if err != nil {
		return Result{}, err
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create process job: %w", err)
	}
	defer func() {
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err != nil {
		return Result{}, fmt.Errorf("configure process job: %w", err)
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		return Result{}, err
	}
	defer outR.Close()
	defer outW.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		return Result{}, err
	}
	defer errR.Close()
	defer errW.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		return Result{}, err
	}
	defer null.Close()
	current, err := windows.GetCurrentProcess()
	if err != nil {
		return Result{}, err
	}
	handles := make([]windows.Handle, 3)
	for i, file := range []*os.File{null, outW, errW} {
		if err = windows.DuplicateHandle(current, windows.Handle(file.Fd()), current, &handles[i], 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			for _, h := range handles {
				if h != 0 {
					windows.CloseHandle(h)
				}
			}
			return Result{}, err
		}
	}
	defer func() {
		for _, h := range handles {
			if h != 0 {
				windows.CloseHandle(h)
			}
		}
	}()
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return Result{}, err
	}
	defer attributes.Delete()
	jobs := []windows.Handle{job}
	if err = attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])); err != nil {
		return Result{}, fmt.Errorf("set process job list: %w", err)
	}
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return Result{}, err
	}
	si := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW, ShowWindow: windows.SW_HIDE, StdInput: handles[0], StdOutput: handles[1], StdErr: handles[2]}, ProcThreadAttributeList: attributes.List()}
	pi := windows.ProcessInformation{}
	started := time.Now().UTC()
	err = windows.CreateProcess(app, &line[0], nil, nil, true, windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT|createNoWindow, &env[0], dir, &si.StartupInfo, &pi)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(env)
	if err != nil {
		return Result{}, fmt.Errorf("start process in job: %w", err)
	}
	defer windows.CloseHandle(pi.Process)
	windows.CloseHandle(pi.Thread)
	// Close every parent copy of the write handles before reading, or EOF never arrives.
	outW.Close()
	errW.Close()
	for i, h := range handles {
		windows.CloseHandle(h)
		handles[i] = 0
	}
	if onEvent != nil {
		onEvent(Event{Type: EventStarted, Timestamp: started})
	}
	stdout := &lineCapture{eventType: EventStdout, onEvent: onEvent, cancel: cancel}
	stderr := &lineCapture{eventType: EventStderr, onEvent: onEvent, cancel: cancel}
	var reads sync.WaitGroup
	reads.Add(2)
	go func() { defer reads.Done(); _, _ = io.Copy(stdout, outR); stdout.finish() }()
	go func() { defer reads.Done(); _, _ = io.Copy(stderr, errR); stderr.finish() }()
	process, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		_ = windows.TerminateJobObject(job, 1)
		return Result{}, fmt.Errorf("open started process: %w", err)
	}
	type waitResult struct {
		state *os.ProcessState
		err   error
	}
	waited := make(chan waitResult, 1)
	go func() { state, err := process.Wait(); waited <- waitResult{state, err} }()
	var waitErr error
	var state *os.ProcessState
	rootWaited := false
	select {
	case result := <-waited:
		state, waitErr = result.state, result.err
		rootWaited = true
	case <-runCtx.Done():
		if e := terminate(job, 1); e != nil {
			waitErr = fmt.Errorf("terminate process job: %w", e)
			// Closing the final job handle is an independent cleanup path.
			_ = windows.CloseHandle(job)
			job = 0
		}
		select {
		case result := <-waited:
			rootWaited = true
			state = result.state
			if waitErr == nil {
				waitErr = result.err
			}
			if waitErr == nil {
				waitErr = runCtx.Err()
			}
		case <-time.After(3 * time.Second):
			if waitErr == nil {
				waitErr = errors.New("process did not exit after job termination")
			}
		}
	}
	// A root may exit while children still hold output pipes; stop remaining job members.
	if job != 0 {
		if e := terminate(job, 1); e != nil {
			if waitErr == nil {
				waitErr = fmt.Errorf("terminate remaining job processes: %w", e)
			}
			_ = windows.CloseHandle(job)
			job = 0
		}
	}
	done := make(chan struct{})
	go func() { reads.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		outR.Close()
		errR.Close()
		<-done
		if waitErr == nil {
			waitErr = exec.ErrWaitDelay
		}
	}
	exitCode := -1
	if rootWaited {
		var code uint32
		if e := windows.GetExitCodeProcess(pi.Process, &code); e != nil {
			if waitErr == nil {
				waitErr = e
			}
		} else {
			exitCode = int(code)
		}
	}
	finished := time.Now().UTC()
	if onEvent != nil {
		onEvent(Event{Type: EventFinished, Timestamp: finished})
	}
	waitErr = errors.Join(waitErr, stdout.err, stderr.err)
	if waitErr == nil && exitCode != 0 {
		waitErr = &exec.ExitError{ProcessState: state}
	}
	return Result{ExitCode: exitCode, Stdout: stdout.output.String(), Stderr: stderr.output.String(), Started: started, Finished: finished, TimedOut: runCtx.Err() == context.DeadlineExceeded}, waitErr
}

// Windows requires a sorted, double-NUL-terminated UTF-16 environment block.
func windowsEnvironment(overrides map[string]string) ([]uint16, error) {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		// Windows also uses hidden drive-current-directory entries such as =C:=C:\dir.
		start := 0
		if strings.HasPrefix(entry, "=") {
			start = 1
		}
		if i := strings.Index(entry[start:], "="); i >= 0 && i+start > 0 {
			values[strings.ToUpper(entry[:i+start])] = entry
		}
	}
	for key, value := range overrides {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("invalid environment variable %q", key)
		}
		values[strings.ToUpper(key)] = key + "=" + value
	}
	entries := make([]string, 0, len(values))
	for _, entry := range values {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToUpper(entries[i]) < strings.ToUpper(entries[j]) })
	block := make([]uint16, 0)
	for _, entry := range entries {
		encoded, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, err
		}
		block = append(block, encoded...)
	}
	block = append(block, 0)
	if len(block) == 1 {
		block = append(block, 0)
	}
	return block, nil
}
