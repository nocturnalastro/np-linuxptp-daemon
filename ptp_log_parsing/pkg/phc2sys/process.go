package phc2sys

import (
	"ptplogparser/pkg/process"
)

const (
	ProcessName = "phc2sys"
	processPath = "/usr/sbin/phc2sys"
)

func NewProcess(lines chan<- string, opts []string) process.Process {
	return process.NewBaseProcess(
		ProcessName, processPath, opts, lines,
	)
}
