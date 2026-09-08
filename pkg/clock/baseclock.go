package clock

import (
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/event"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/ipc"
	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/utils"
)

// BaseClock ...
type BaseClock struct {
	sendIPC          func(ipc.Message)
	cfgName          string
	data             []*event.Data
	overallSyncState event.PTPState
	osClock          OsClock
}

// ConfigName ..
func (c *BaseClock) ConfigName() string { return c.cfgName }

// Reset resets the clock state.
func (c *BaseClock) Reset() {
	c.osClock.Reset()
	c.overallSyncState = event.PTP_FREERUN
	c.data = nil
}

// GetData returns the Data entry for the given process, creating one if needed.
func (c *BaseClock) GetData(processName event.EventSource) *event.Data {
	for _, d := range c.data {
		if d.ProcessName == processName {
			return d
		}
	}
	d := &event.Data{ProcessName: processName, State: event.PTP_UNKNOWN, Window: *utils.NewWindow(event.WindowSize)}
	c.data = append(c.data, d)
	return d
}

// ProcessData returns all clock data accumulated from processed events.
func (c *BaseClock) ProcessData() []*event.Data {
	return c.data
}

// OsClock ...
type OsClock struct {
	State event.PTPState
}

// Reset ...
func (o *OsClock) Reset() {
	o.State = event.PTP_FREERUN
}
