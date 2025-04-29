package ptp4l

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

type Ptp4lParser struct {
	process process.Process
	inChan  <-chan string
	outChan chan<- events.Event
	quit    chan struct{}
	wg      sync.WaitGroup
}

func NewParser(inChan <-chan string, outchan chan<- events.Event, process process.Process) *Ptp4lParser {
	return &Ptp4lParser{
		process: process,
		inChan:  inChan,
		outChan: outchan,
		quit:    make(chan struct{}),
	}
}

func (p *Ptp4lParser) Start() {
	go p.parse()
}

func (p *Ptp4lParser) Stop(wait bool) {
	p.quit <- struct{}{}
	p.process.Stop()
	if wait {
		p.wg.Wait()
	}
}

func (p *Ptp4lParser) parse() {
	p.wg.Add(1)
	defer p.wg.Done()
	for {
		select {
		case <-p.quit:
			return
		default:
			time.Sleep(time.Nanosecond)
		case line := <-p.inChan:
			event, err, triedToParse := parseLine(line)
			if !triedToParse {
				continue
			}

			if err != nil {
				fmt.Println(fmt.Errorf("failed to parse ptp4l line: %s", err))
			}
			p.outChan <- event
		}
	}
}

const (
	metricOffsetField = 5
	metricStateField  = 6
	metricFreqField   = 8
	metricDelayField  = 11
)

var (
	replacer = strings.NewReplacer("[", " ", "]", " ", ":", " ", "(", " ", ")", " ")
)

func parseLine(line string) (events.Event, error, bool) {
	line = replacer.Replace(line)
	fields := strings.Fields(line)
	if strings.Contains(line, "offset") {
		// Figure out which one of the parsers to use
		evt, err := parseMetricLine(fields)
		return evt, err, true
	} else if strings.Contains(line, "port") {
		evt, err := parsePortLine(fields)
		return evt, err, true
	}
	return nil, nil, false
}

func parseMetricLine(fields []string) (events.Event, error) {
	// 0     1           2              3      4      5        6  7    8         9    10    11
	// ptp4l 5196819.100 ptp4l.0.config master offset -2162130 s2 freq +22451884 path delay 374976
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
	return &Ptp4lEvent{
		State: ptp.StateFromPtp4l(fields[metricStateField]),
		// Interface: fields[interfaceField],
		Offset:    offset,
		Freq:      freq,
		PathDelay: delay,
	}, nil
}

type Ptp4lEvent struct {
	State  ptp.State
	Offset int64
	// Interface string
	Freq      int64
	PathDelay int64
}

func (e *Ptp4lEvent) SubType() events.EventType {
	return events.PTPMeteric
}

func (e *Ptp4lEvent) Marshal() ([]byte, error) {
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

func (e *Ptp4lEvent) Value() map[string]any {
	return map[string]any{
		"state":  e.State,
		"offset": e.Offset,
		// "interface": e.Interface,
		"freq":      e.Freq,
		"pathdelay": e.PathDelay,
	}
}

const (
	portInterfaceField    = 6
	portRoleField         = 9
	portPreviousRoleField = 7
	portActionField       = 11
)

func parsePortLine(fields []string) (events.Event, error) {
	// 0     1           2              3 4    5 6      7         8  9      10 11                               12
	// ptp4l 1361315.822 ptp4l.0.config 5 port 1 ens4f0 MASTER    to FAULTY on FAULT_DETECTED                   FT_UNSPECIFIED
	// ptp4l 1361316.340 ptp4l.0.config 5 port 1 ens4f0 LISTENING to MASTER on ANNOUNCE_RECEIPT_TIMEOUT_EXPIRES
	return &Ptp4lRoleEvent{
		Interface:    fields[portInterfaceField],
		Role:         ptp.PortRoleFromPtp4l(fields[portRoleField]),
		PreviousRole: ptp.PortRoleFromPtp4l(fields[portPreviousRoleField]),
		Action:       ptp.RoleActionFromPtp4l(fields[portActionField]),
	}, nil

}

type Ptp4lRoleEvent struct {
	Interface    string
	Role         ptp.PortRole
	PreviousRole ptp.PortRole
	Action       ptp.RoleAction
}

func (e Ptp4lRoleEvent) SubType() events.EventType {
	return events.PortRole
}

func (e *Ptp4lRoleEvent) Marshal() ([]byte, error) {
	return json.Marshal(struct {
		Interface    string `json:"interface"`
		Role         string `json:"role"`
		PreviousRole string `json:"previousRole"`
		Action       string `json:"action"`
	}{
		Interface:    e.Interface,
		Role:         e.Role.String(),
		PreviousRole: e.PreviousRole.String(),
		Action:       e.Action.String(),
	})
}

func (e *Ptp4lRoleEvent) Value() map[string]any {
	return map[string]any{
		"interface":    e.Interface,
		"role":         e.Role.String(),
		"previousRole": e.PreviousRole.String(),
		"action":       e.Action.String(),
	}
}
