package ptp4l

import (
	"encoding/json"
	"ptplogparser/pkg/consts"
	"ptplogparser/pkg/events"
	"ptplogparser/pkg/parser"
	"ptplogparser/pkg/process"
	"ptplogparser/pkg/ptp"
	"strconv"
	"strings"
)

const PTP4LName = "ptp4l"

func NewParser(inChan <-chan string, outchan chan<- events.Event, process process.Process) parser.Parser {
	return parser.NewParser(
		PTP4LName,
		inChan,
		outchan,
		process,
		parseLine,
	)
}

const (
	metricInterfaceField = 3
	metricOffsetField    = 5
	metricStateField     = 6
	metricFreqField      = 8
	metricDelayField     = 11
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
	return &Ptp4lOffsetEvent{
		State:     ptp.StateFromString(fields[metricStateField]),
		Interface: fields[metricInterfaceField],
		Offset:    offset,
		Freq:      freq,
		PathDelay: delay,
	}, nil
}

type Ptp4lOffsetEvent struct {
	State     ptp.State
	Offset    int64
	Interface string
	Freq      int64
	PathDelay int64
}

func (e *Ptp4lOffsetEvent) SubType() events.EventType {
	return events.Ptp4lOffsetEvent
}

func (e *Ptp4lOffsetEvent) Marshal() ([]byte, error) {
	return json.Marshal(struct {
		State     string `json:"state"`
		Offset    int64  `json:"offset"`
		Interface string `json:"interface"`
		Freq      int64  `json:"freq"`
		PathDelay int64  `json:"pathDelay"`
	}{
		State:     e.State.String(),
		Offset:    e.Offset,
		Interface: e.Interface,
		Freq:      e.Freq,
		PathDelay: e.PathDelay,
	})
}

func (e *Ptp4lOffsetEvent) Value() map[string]any {
	return map[string]any{
		"state":     e.State,
		"offset":    e.Offset,
		"interface": e.Interface,
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
		consts.InterfaceKey:    e.Interface,
		consts.RoleKey:         e.Role.String(),
		consts.PreviousRoleKey: e.PreviousRole.String(),
		consts.ActionKey:       e.Action.String(),
	}
}
