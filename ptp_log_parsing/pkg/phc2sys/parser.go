package phc2sys

import (
	"encoding/json"
	"fmt"
	"ptplogparser/pkg/events"
	"ptplogparser/pkg/process"
	"ptplogparser/pkg/ptp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Phc2SysParser struct {
	process process.Process
	inChan  <-chan string
	outChan chan<- events.Event
	quit    chan struct{}
	wg      sync.WaitGroup
}

func NewParser(inChan <-chan string, outchan chan<- events.Event, process process.Process) *Phc2SysParser {
	return &Phc2SysParser{
		process: process,
		inChan:  inChan,
		outChan: outchan,
		quit:    make(chan struct{}),
	}
}

func (p *Phc2SysParser) Start() {
	go p.parse()
}

func (p *Phc2SysParser) Stop(wait bool) {
	p.quit <- struct{}{}
	p.process.Stop()
	if wait {
		p.wg.Wait()
	}
}

func (p *Phc2SysParser) parse() {
	p.wg.Add(1)
	defer p.wg.Done()
	for {
		select {
		case <-p.quit:
			return
		default:
			time.Sleep(time.Nanosecond)
		case line := <-p.inChan:
			event, err := parseLine(line)
			if err != nil {
				fmt.Println(fmt.Errorf("failed to parse ptp4l line: %s", err))
			}
			p.outChan <- event
		}
	}
}

const (
	metricOffsetField = 7
	metricStateField  = 8
	metricFreqField   = 10
	metricDelayField  = 12
)

var (
	replacer = strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ")
)

func parseLine(line string) (events.Event, error) {
	line = replacer.Replace(line)
	fields := strings.Fields(line)
	// 0       1           2              3 4              5   6      7        8  9    10      11    12
	// phc2sys 1361551.425 ptp4l.0.config 6 CLOCK_REALTIME phc offset -1104133 s2 freq +287746 delay 496
	offset, err := strconv.ParseInt(fields[metricOffsetField], 10, 64)
	if err != nil {
		return nil, err
	}
	freq, err := strconv.ParseInt(fields[metricFreqField], 10, 64)
	if err != nil {
		return nil, err
	}
	delay, err := strconv.ParseInt(fields[metricDelayField], 10, 64)
	if err != nil {
		return nil, err
	}
	return &Phc2SysEvent{
		State: ptp.StateFromPtp4l(fields[metricStateField]),
		// Interface: fields[interfaceField],
		Offset:    offset,
		Freq:      freq,
		PathDelay: delay,
	}, nil
}

type Phc2SysEvent struct {
	State  ptp.State
	Offset int64
	// Interface string
	Freq      int64
	PathDelay int64
}

func (e *Phc2SysEvent) SubType() events.EventType {
	return events.Phc2SysMetric
}

func (e *Phc2SysEvent) Marshal() ([]byte, error) {
	return json.Marshal(struct {
		State  string `json:"state"`
		Offset int64  `json:"offset"`
		// Interface string `json:"interface"`
		Freq      int64 `json:"freq"`
		PathDelay int64 `json:"pathDelay"`
	}{
		State:  e.State.String(),
		Offset: e.Offset,
		// Interface: e.Interface,
		Freq:      e.Freq,
		PathDelay: e.PathDelay,
	})
}

func (e *Phc2SysEvent) Value() map[string]any {
	return map[string]any{
		"state":  e.State,
		"offset": e.Offset,
		// "interface": e.Interface,
		"freq":      e.Freq,
		"pathdelay": e.PathDelay,
	}
}
