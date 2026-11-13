package gg

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func darwinProcess(p unix.KinfoProc) processInfo {
	return processInfo{pid: int(p.Proc.P_pid), parent: int(p.Eproc.Ppid), start: fmt.Sprintf("%d:%d", p.Proc.P_starttime.Sec, p.Proc.P_starttime.Usec)}
}
func readProcess(pid int) (processInfo, bool) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return processInfo{}, false
	}
	return darwinProcess(*p), true
}
func processTable() (map[int]processInfo, error) {
	all, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	out := map[int]processInfo{}
	for _, p := range all {
		info := darwinProcess(p)
		out[info.pid] = info
	}
	return out, nil
}
