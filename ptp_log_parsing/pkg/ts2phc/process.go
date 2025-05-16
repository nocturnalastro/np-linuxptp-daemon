package ts2phc

import (
	"ptplogparser/pkg/process"
)

const (
	ProcessName = "phc2sys"
	processPath = "/usr/sbin/ts2phc"
)

func NewProcess(lines chan<- string, opts []string) process.Process {
	return process.NewBaseProcess(
		ProcessName, processPath, opts, lines,
	)
}
