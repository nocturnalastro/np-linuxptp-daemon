package ts2phc

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

type Ts2PhcParser struct {
	process process.Process
	inChan  <-chan string
	outChan chan<- events.Event
	quit    chan struct{}
	wg      sync.WaitGroup
}

func NewParser(inChan <-chan string, outchan chan<- events.Event, process process.Process) *Ts2PhcParser {
	return &Ts2PhcParser{
		process: process,
		inChan:  inChan,
		outChan: outchan,
		quit:    make(chan struct{}),
	}
}

func (p *Ts2PhcParser) Start() {
	go p.parse()
}

func (p *Ts2PhcParser) Stop(wait bool) {
	p.quit <- struct{}{}
	p.process.Stop()
	if wait {
		p.wg.Wait()
	}
}

func (p *Ts2PhcParser) parse() {
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
	metricInterfaceField = 4
	metricOffsetField    = 6
	metricStateField     = 7
	metricFreqField      = 9
)

var (
	replacer = strings.NewReplacer("master", " ")
)

// example lines
// ts2phc[82674.465]:    [ts2phc.0.config]   ens2f1 master  offset          0 s2 freq      -0
// ts2phc[521734.693]:   [ts2phc.0.config:6] /dev/ptp6      offset          0 s2 freq      -0

// ignored lines
// ts2phc[82674.465]:    [ts2phc.0.config]   nmea delay: 88403525 ns
// ts2phc[82674.465]:    [ts2phc.0.config]   ens2f1 extts index 0 at 1673031129.000000000 corr 0 src 1673031129.911642976 diff 0

func parseLine(line string) (events.Event, error, bool) {
	if strings.Contains(line, "offset") {
		evt, err := parseOffsetMetric(line)
		return evt, err, true
	}
	return nil, nil, false
}

func parseOffsetMetric(line string) (events.Event, error) {
	fields := strings.Fields(line)
	line = replacer.Replace(line)
	// Fields after replace and split
	// 1                  3                   4         5      6 7  8     9
	// ts2phc[82674.465]: [ts2phc.0.config]   ens2f1    offset 0 s2 freq -0
	// ts2phc[521734.693]:[ts2phc.0.config:6] /dev/ptp6 offset 0 s2 freq -0

	offset, err := strconv.ParseInt(fields[metricOffsetField], 10, 64)
	if err != nil {
		return nil, err
	}
	freq, err := strconv.ParseInt(fields[metricFreqField], 10, 64)
	if err != nil {
		return nil, err
	}
	return &Ts2PhcOffset{
		State:     ptp.StateFromString(fields[metricStateField]),
		Interface: fields[metricInterfaceField], // TODO look up interface by clock device
		Offset:    offset,
		Freq:      freq,
	}, nil
}

type Ts2PhcOffset struct {
	State     ptp.State
	Offset    int64
	Interface string
	Freq      int64
}

func (e *Ts2PhcOffset) SubType() events.EventType {
	return events.Ts2phcOffset
}

func (e *Ts2PhcOffset) Marshal() ([]byte, error) {
	return json.Marshal(struct {
		State     string `json:"state"`
		Offset    int64  `json:"offset"`
		Interface string `json:"interface"`
		Freq      int64  `json:"freq"`
	}{
		State:     e.State.String(),
		Offset:    e.Offset,
		Interface: e.Interface,
		Freq:      e.Freq,
	})
}

func (e *Ts2PhcOffset) Value() map[string]any {
	return map[string]any{
		"state":     e.State,
		"offset":    e.Offset,
		"interface": e.Interface,
		"freq":      e.Freq,
	}
}
