package ublox

import (
	"bufio"
	"fmt"
	"os/exec"
	"ptplogparser/pkg/process"
	"regexp"
	"sync"
	"time"

	"github.com/golang/glog"
)

const (
	pollWait      = 1000000000
	allowedMissed = 3
	PollInterval  = 1 * time.Second
	PopTimeout    = 1 * time.Microsecond
)

var (
	protoVerRegex = regexp.MustCompile(`PROTVER=+(\d+)`)
)

type UbxProcess struct {
	ubxtoolPath string

	statusMutex sync.RWMutex
	status      process.Status

	lines  chan<- string
	cmd    *exec.Cmd
	reader *bufio.Reader
}

func NewProcess(lines chan<- string) process.Process {
	return &UbxProcess{
		ubxtoolPath: "/usr/local/bin/ubxtool",
		status:      process.New,
		lines:       lines,
	}
}

func (u *UbxProcess) Name() string {
	return "ublox"
}

func (u *UbxProcess) Status() process.Status {
	u.statusMutex.RLock()
	defer u.statusMutex.RUnlock()
	return u.status
}

func (u *UbxProcess) setStatus(val process.Status) {
	u.statusMutex.Lock()
	u.status = val
	u.statusMutex.Unlock()
}

func (u *UbxProcess) Start() error {
	if u.Status() != process.New && u.Status() != process.Dead {
		return nil
	}
	err := u.start()
	if err != nil {
		return err
	}
	go u.processReads()
	return nil
}

func (u *UbxProcess) Reset() {
	pid := u.cmd.Process.Pid
	glog.Infof("Stopping ubxtool polling with PID=%d", pid)
	_ = u.cmd.Process.Kill()
	if u.Status() != process.Stopped {
		u.setStatus(process.Dead)
	}
	u.cmd.Wait()
	u.start()
}

func (u *UbxProcess) Stop() error {
	pid := u.cmd.Process.Pid
	glog.Infof("Stopping ubxtool polling with PID=%d", pid)
	u.setStatus(process.Stopped)
	_ = u.cmd.Process.Kill()
	u.cmd.Wait()
	return nil
}

func (u *UbxProcess) start() error {
	wait := fmt.Sprintf("%d", pollWait)
	fmt.Println("python3", "-u", u.ubxtoolPath, "-t", "-P", "29.20", "-w", wait)
	u.cmd = exec.Command("python3", "-u", u.ubxtoolPath, "-t", "-P", "29.20", "-w", wait)
	stdoutreader, _ := u.cmd.StdoutPipe()
	// u.reader = bufio.NewReader(io.TeeReader(stdoutreader, os.Stdout))
	u.reader = bufio.NewReader(stdoutreader)
	err := u.cmd.Start()
	if err != nil {
		glog.Errorf("Failed to start ublox poll: %s", err.Error())
		u.setStatus(process.Stopped)
		return fmt.Errorf("failed to start ublox poll: %w", err)
	} else {
		u.setStatus(process.Running)
		pid := u.cmd.Process.Pid
		glog.Infof("Starting ubxtool polling with PID=%d", pid)
	}
	return nil
}

const timeLsResultLines = 4

func (u *UbxProcess) processReads() {
	for {
		scanner := bufio.NewScanner(u.reader)
		for scanner.Scan() {

			line := scanner.Text()
			if line == "" {
				continue
			}
			u.lines <- line
		}

		if err := scanner.Err(); err != nil {
			glog.Errorf("ublox poll thread error %s", err)
		} else {
			// glog.Errorf("ublox poll exited with EOF, will retry")
			continue
		}
		time.Sleep(time.Nanosecond)
	}
}
