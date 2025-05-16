package ublox

import (
	"fmt"
	"ptplogparser/pkg/process"
	"regexp"
	"strings"
)

const (
	pollWait = 1000000000

	ubxtoolPath = "/usr/local/bin/ubxtool"
	ProcessName = "ubxtool"
)

var (
	protoVerRegex = regexp.MustCompile(`PROTVER=+(\d+)`)
)

func NewProcess(lines chan<- string) process.Process {
	cmdSplit := strings.Fields(
		fmt.Sprintf("python3 -u %s -t -P 29.20 -w %d", ubxtoolPath, pollWait),
	)
	return process.NewBaseProcess(
		ProcessName, cmdSplit[0], cmdSplit[1:], lines,
	)
}
