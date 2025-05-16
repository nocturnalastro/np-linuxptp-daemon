package phc2sys

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

const Phc2SysName = "Phc2Sys"

func NewParser(inChan <-chan string, outchan chan<- events.Event, process process.Process) parser.Parser {
	return parser.NewParser(
		Phc2SysName,
		inChan,
		outchan,
		process,
		parseLine,
	)
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

func parseLine(line string) (events.Event, error, bool) {
	line = replacer.Replace(line)
	fields := strings.Fields(line)
	// 0       1           2              3 4              5   6      7        8  9    10      11    12
	// phc2sys 1361551.425 ptp4l.0.config 6 CLOCK_REALTIME phc offset -1104133 s2 freq +287746 delay 496
	offset, err := strconv.ParseInt(fields[metricOffsetField], 10, 64)
	if err != nil {
		return nil, err, true
	}
	freq, err := strconv.ParseInt(fields[metricFreqField], 10, 64)
	if err != nil {
		return nil, err, true
	}
	delay, err := strconv.ParseInt(fields[metricDelayField], 10, 64)
	if err != nil {
		return nil, err, true
	}
	evt := &Phc2SysEvent{
		State:     ptp.StateFromString(fields[metricStateField]),
		Offset:    offset,
		Freq:      freq,
		PathDelay: delay,
	}
	return evt, nil, true
}

type Phc2SysEvent struct {
	State     ptp.State
	Offset    int64
	Freq      int64
	PathDelay int64
}

func (e *Phc2SysEvent) SubType() events.EventType {
	return events.Phc2SysMetric
}

func (e *Phc2SysEvent) Marshal() ([]byte, error) {
	return json.Marshal(struct {
		State     string `json:"state"`
		Offset    int64  `json:"offset"`
		Freq      int64  `json:"freq"`
		PathDelay int64  `json:"pathDelay"`
	}{
		State:     e.State.String(),
		Offset:    e.Offset,
		Freq:      e.Freq,
		PathDelay: e.PathDelay,
	})
}

func (e *Phc2SysEvent) Value() map[string]any {
	return map[string]any{
		consts.StateKey:     e.State,
		consts.OffsetKey:    e.Offset,
		consts.FreqKey:      e.Freq,
		consts.PathdelayKey: e.PathDelay,
	}
}
