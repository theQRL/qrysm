// Copyright 2023 Martin Holst Swende
// This file is part of the goevmlab library.
//
// The library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the goevmlab library. If not, see <http://www.gnu.org/licenses/>.

package qrvms

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// The GqrlBatchVM spins up one 'master' instance of the VM, and uses that to execute tests
type GqrlBatchVM struct {
	GqrlQRVM
	cmd    *exec.Cmd // the 'master' process
	stdout io.ReadCloser
	stdin  io.WriteCloser
	mu     sync.Mutex
}

func NewGqrlBatchVM(path, name string) *GqrlBatchVM {
	return &GqrlBatchVM{
		GqrlQRVM: GqrlQRVM{path, name, &VmStat{}},
	}
}

func (qrvm *GqrlBatchVM) Instance(threadId int) Qrvm {
	return &GqrlBatchVM{
		GqrlQRVM: GqrlQRVM{
			path:  qrvm.path,
			name:  fmt.Sprintf("%v-%d", qrvm.name, threadId),
			stats: qrvm.stats,
		},
	}
}

// RunStateTest implements the Evm interface
func (qrvm *GqrlBatchVM) RunStateTest(path string, out io.Writer, speedTest bool) (*tracingResult, error) {
	var (
		t0     = time.Now()
		err    error
		cmd    *exec.Cmd
		stdout io.ReadCloser
		stdin  io.WriteCloser
	)
	if qrvm.cmd == nil {
		if speedTest {
			cmd = exec.Command(qrvm.path, "--nomemory", "--noreturndata", "--nostack", "statetest")
		} else {
			cmd = exec.Command(qrvm.path, "--json", "--noreturndata", "--nomemory", "statetest")
		}
		if stdout, err = cmd.StderrPipe(); err != nil {
			return &tracingResult{Cmd: cmd.String()}, err
		}
		if stdin, err = cmd.StdinPipe(); err != nil {
			return &tracingResult{Cmd: cmd.String()}, err
		}
		if err = cmd.Start(); err != nil {
			return &tracingResult{Cmd: cmd.String()}, err
		}
		qrvm.cmd = cmd
		qrvm.stdout = stdout
		qrvm.stdin = stdin
	}
	qrvm.mu.Lock()
	defer qrvm.mu.Unlock()
	_, _ = qrvm.stdin.Write(fmt.Appendf(nil, "%v\n", path))
	// copy everything for the _current_ statetest to the given writer
	qrvm.copyUntilEnd(out, qrvm.stdout)
	// release resources, handle error but ignore non-zero exit codes
	duration, slow := qrvm.stats.TraceDone(t0)
	return &tracingResult{
			Slow:     slow,
			ExecTime: duration,
			Cmd:      qrvm.cmd.String()},
		nil
}

func (qrvm *GqrlBatchVM) Close() {
	if qrvm.stdin != nil {
		qrvm.stdin.Close()
	}
	if qrvm.cmd != nil {
		_ = qrvm.cmd.Wait()
	}
}

func (qrvm *GqrlBatchVM) GetStateRoot(path string) (root, command string, err error) {
	if qrvm.cmd == nil {
		qrvm.cmd = exec.Command(qrvm.path)
		if qrvm.stdout, err = qrvm.cmd.StdoutPipe(); err != nil {
			return "", qrvm.cmd.String(), err
		}
		if qrvm.stdin, err = qrvm.cmd.StdinPipe(); err != nil {
			return "", qrvm.cmd.String(), err
		}
		if err = qrvm.cmd.Start(); err != nil {
			return "", qrvm.cmd.String(), err
		}
	}
	qrvm.mu.Lock()
	defer qrvm.mu.Unlock()
	_, _ = qrvm.stdin.Write(fmt.Appendf(nil, "%v\n", path))
	sRoot := qrvm.copyUntilEnd(io.Discard, qrvm.stdout)
	return sRoot.StateRoot, qrvm.cmd.String(), nil
}
