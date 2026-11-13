package gg

import (
	"os"
	"strconv"
	"strings"
)

func readProcess(pid int) (processInfo, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return processInfo{}, false
	}
	end := strings.LastIndexByte(string(b), ')')
	if end < 0 {
		return processInfo{}, false
	}
	f := strings.Fields(string(b)[end+1:])
	if len(f) < 20 {
		return processInfo{}, false
	}
	parent, _ := strconv.Atoi(f[1])
	return processInfo{pid: pid, parent: parent, start: f[19]}, true
}

func processTable() (map[int]processInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	processes := map[int]processInfo{}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if p, ok := readProcess(pid); ok {
			processes[pid] = p
		}
	}
	return processes, nil
}
