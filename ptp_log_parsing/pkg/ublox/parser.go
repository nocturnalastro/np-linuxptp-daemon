package ublox

import (
	"encoding/json"
	"fmt"
	"ptplogparser/pkg/events"
	"ptplogparser/pkg/process"
	"strconv"
	"strings"
	"sync"
	"time"
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

type UbxParser struct {
	process *UbxProcess
	inChan  <-chan string
	outChan chan<- events.Event
	quit    chan bool

	valuesMutex sync.Mutex
	values      InstantValues
	wg          sync.WaitGroup
}

func NewParser(inChan <-chan string, outChan chan<- events.Event, process *process.Process) *UbxParser {
	return &UbxParser{
		inChan:  inChan,
		outChan: outChan,
		quit:    make(chan bool, 2),
	}
}

func (p *UbxParser) Start() {
	p.values.Reset()
	p.process.Start()
	go p.parse()
	go p.processEvents()
}

func (p *UbxParser) Stop(wait bool) {
	p.quit <- true
	p.quit <- true
	p.process.Stop()
	if wait {
		p.wg.Wait()
	}

}

func (p *UbxParser) parse() {
	p.wg.Add(1)
	defer p.wg.Done()
	for {
		select {
		case line := <-p.inChan:
			if strings.Contains(line, "UBX-NAV-CLOCK") {
				nextLine := <-p.inChan
				p.values.SetOffset(extractOffset(nextLine))
			} else if strings.Contains(line, "UBX-NAV-STATUS") {
				nextLine := <-p.inChan
				p.values.SetGPSFix(extractNavStatus(nextLine))
			} else if strings.Contains(line, "UBX-NAV-TIMELS") {
				var lines []string
				for i := 0; i < timeLsResultLines; i++ {
					nextLine := <-p.inChan
					lines = append(lines, nextLine)
				}
				p.values.SetTimeLs(extractLeapSec(lines))
			}
		case <-p.quit:
			return
		default:
			time.Sleep(time.Nanosecond)
		}
	}
}

type UBloxEvent struct {
	GPSFix int8    `json:"gpsFix"`
	Offset int64   `json:"offset"`
	TimeLs *TimeLs `json:"timeLs"`
}

func (e *UBloxEvent) SubType() events.EventType {
	return events.GNSSMetric
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

func (p *UbxParser) processEvents() {
	p.wg.Add(1)
	defer p.wg.Done()

	ticker := time.NewTicker(PollInterval)
	missedTicks := 0
	for {
		select {
		case <-ticker.C:
			if p.values.IsStale() {
				missedTicks++
				if missedTicks > allowedMissed {
					p.process.Reset()
					missedTicks = 0
				}
				continue
			} else {
				missedTicks = 0
			}

			p.valuesMutex.Lock()
			event := UBloxEvent{
				GPSFix: p.values.GPSFix,
				Offset: p.values.Offset,
				TimeLs: p.values.TimeLs,
			}
			p.valuesMutex.Unlock()
			p.values.Reset()

			p.outChan <- &event
		case <-p.quit:
			return
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
