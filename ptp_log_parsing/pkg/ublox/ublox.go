package ublox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"ptplogparser/pkg/process"
	"regexp"
	"strconv"
	"strings"
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

type InstantValues struct {
	Offset int64
	GPSFix int8
	TimeLs *TimeLs
	stale  bool
	lock   sync.Mutex
}

func (v *InstantValues) IsStale() bool {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.stale
}

func (v *InstantValues) Reset() {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.stale = true
	v.Offset = 0
	v.GPSFix = 0
	v.TimeLs = nil
}
func (v *InstantValues) SetOffset(offset int64) {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.Offset = offset
	v.stale = false
}

func (v *InstantValues) SetGPSFix(fix int8) {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.GPSFix = fix
	v.stale = false
}

func (v *InstantValues) SetTimeLs(timeLs *TimeLs) {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.TimeLs = timeLs
	v.stale = false
}

type UbxProcess struct {
	ubxtoolPath string

	statusMutex sync.RWMutex
	status      process.Status

	ch     chan<- process.Event
	cmd    *exec.Cmd
	reader *bufio.Reader

	valuesMutex sync.Mutex
	values      InstantValues

	missedTicks int8
}

func New(ch chan<- process.Event) process.Process {

	return &UbxProcess{
		ubxtoolPath: "/usr/local/bin/ubxtool",
		status:      process.New,
		ch:          ch,
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
	u.values.Reset()

	if u.Status() != process.New && u.Status() != process.Dead {
		return nil
	}
	err := u.start()
	if err != nil {
		return err
	}
	go u.processReads()
	go u.processEvents()
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
			// fmt.Println(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, "UBX-NAV-CLOCK") {
				if !scanner.Scan() {
					break
				}
				u.values.SetOffset(extractOffset(scanner.Text()))
			} else if strings.Contains(line, "UBX-NAV-STATUS") {
				if !scanner.Scan() {
					break
				}
				u.values.SetGPSFix(extractNavStatus(scanner.Text()))
			} else if strings.Contains(line, "UBX-NAV-TIMELS") {
				var lines []string
				for i := 0; i < timeLsResultLines; i++ {
					if !scanner.Scan() {
						goto doubleBreak
					}
					lines = append(lines, scanner.Text())
				}
				u.values.SetTimeLs(extractLeapSec(lines))
			}

		}
	doubleBreak:
		if err := scanner.Err(); err != nil {
			glog.Errorf("ublox poll thread error %s", err)
		} else {
			// glog.Errorf("ublox poll exited with EOF, will retry")
			continue
		}
	}
}

type UBloxEvent struct {
	GPSFix int8    `json:"gpsFix"`
	Offset int64   `json:"offset"`
	TimeLs *TimeLs `json:"timeLs"`
}

func (e *UBloxEvent) SubType() process.EventType {
	return process.GNSSMetric
}

func (e *UBloxEvent) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e *UBloxEvent) Value() map[string]any {
	return map[string]any{
		"gpsFix": e.GPSFix,
		"offset": e.Offset,
		"timeLs": e.TimeLs,
	}
}

func (u *UbxProcess) processEvents() {
	ticker := time.NewTicker(PollInterval)
	missedTicks := 0

	for {
		select {
		case <-ticker.C:
			if u.values.IsStale() {
				missedTicks++
				if missedTicks > allowedMissed {
					u.Reset()
					missedTicks = 0
				}
				continue
			} else {
				missedTicks = 0
			}

			u.valuesMutex.Lock()
			event := UBloxEvent{
				GPSFix: u.values.GPSFix,
				Offset: u.values.Offset,
				TimeLs: u.values.TimeLs,
			}
			u.valuesMutex.Unlock()
			u.values.Reset()

			fmt.Println(event)
		}
	}
}

func extractOffset(output string) int64 {
	// Find the line that contains "tAcc"
	// fmt.Println(output)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "tAcc") {
			// Extract the offset value
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "tAcc" {
					ret, _ := strconv.ParseInt(fields[i+1], 10, 64)
					return ret
				}
			}
		}
	}

	return -1
}

func extractNavStatus(output string) int8 {
	// Find the line that contains "gpsFix"
	// fmt.Println(output)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "gpsFix") {
			// Extract the offset value
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "gpsFix" {
					ret, _ := strconv.ParseInt(fields[i+1], 10, 8)
					return int8(ret)
				}
			}
		}
	}
	return -1
}

type TimeLs struct {
	//Information source for the current number
	// of leap seconds
	SrcOfCurrLs uint8
	// Current number of leap seconds since
	// start of GPS time (Jan 6, 1980). It reflects
	// how much GPS time is ahead of UTC time.
	// Galileo number of leap seconds is the
	// same as GPS. BeiDou number of leap
	// seconds is 14 less than GPS. GLONASS
	// follows UTC time, so no leap seconds
	CurrLs int8
	// Information source for the future leap
	// second event.
	SrcOfLsChange uint8
	// Future leap second change if one is
	// scheduled. +1 = positive leap second, -1 =
	// negative leap second, 0 = no future leap
	// second event scheduled or no information
	// available. If the value is 0, then the
	// amount of leap seconds did not change
	// and the event should be ignored
	LsChange int8
	// Number of seconds until the next leap
	// second event, or from the last leap second
	// event if no future event scheduled. If > 0
	// event is in the future, = 0 event is now, < 0
	// event is in the past. Valid only if
	// validTimeToLsEvent = 1
	TimeToLsEvent int
	// GPS week number (WN) of the next leap
	// second event or the last one if no future
	// event scheduled. Valid only if
	// validTimeToLsEvent = 1.
	DateOfLsGpsWn uint
	// GPS day of week number (DN) for the next
	// leap second event or the last one if no
	// future event scheduled. Valid only if
	// validTimeToLsEvent = 1. (GPS and Galileo
	// DN: from 1 = Sun to 7 = Sat. BeiDou DN:
	// from 0 = Sun to 6 = Sat.
	DateOfLsGpsDn uint8
	// Validity flags
	// 1<<0 validCurrLs 1 = Valid current number of leap seconds value.
	// 1<<1 validTimeToLsEvent 1 = Valid time to next leap second event
	// or from the last leap second event if no future event scheduled.
	Valid uint8
}

func extractLeapSec(output []string) *TimeLs {
	var data = TimeLs{}
	for _, line := range output {
		fields := strings.Fields(line)
		for i, field := range fields {
			switch field {
			case "srcOfCurrLs":
				tmp, _ := strconv.ParseUint(fields[i+1], 10, 8)
				data.SrcOfCurrLs = uint8(tmp)
			case "currLs":
				tmp, _ := strconv.ParseInt(fields[i+1], 10, 8)
				data.CurrLs = int8(tmp)
			case "srcOfLsChange":
				tmp, _ := strconv.ParseUint(fields[i+1], 10, 8)
				data.SrcOfLsChange = uint8(tmp)
			case "lsChange":
				tmp, _ := strconv.ParseInt(fields[i+1], 10, 8)
				data.LsChange = int8(tmp)
			case "timeToLsEvent":
				tmp, _ := strconv.ParseInt(fields[i+1], 10, 32)
				data.TimeToLsEvent = int(tmp)
			case "dateOfLsGpsWn":
				tmp, _ := strconv.ParseUint(fields[i+1], 10, 16)
				data.DateOfLsGpsWn = uint(tmp)
			case "dateOfLsGpsDn":
				tmp, _ := strconv.ParseUint(fields[i+1], 10, 16)
				data.DateOfLsGpsDn = uint8(tmp)
			case "valid":
				tmp, _ := strconv.ParseUint(fmt.Sprintf("0%s", fields[i+1]), 0, 8)
				data.Valid = uint8(tmp)
			}
		}
	}
	return &data
}
