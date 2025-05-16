package ptp4l

import (
	"ptplogparser/pkg/process"
)

const (
	ProcessName = "ptp4l"
	processPath = "/usr/sbin/ptp4l"
)

func NewProcess(lines chan<- string, opts []string) process.Process {
	return process.NewBaseProcess(
		ProcessName, processPath, opts, lines,
	)
}
