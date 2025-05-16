package ptp4l_test

import (
	"fmt"
	"ptplogparser/pkg/consts"
	"ptplogparser/pkg/events"
	"ptplogparser/pkg/ptp"
	"ptplogparser/pkg/ptp4l"
	testHelpers "ptplogparser/pkg/testing"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewParser(t *testing.T) {
	// Create a mock channel for input
	inChan := make(chan string, 1)

	// Create a mock channel for output
	outChan := make(chan events.Event, 1)

	// Create a mock process
	mockProcess := &testHelpers.MockProcess{}

	// Call the NewParser function
	parser := ptp4l.NewParser(inChan, outChan, mockProcess)
	assert.NoError(t, parser.Start())

	// Run as sub test
	t.Run(
		"Faulty port",
		func(t *testing.T) {

			ifname := "ens4f0"
			role := ptp.FaultyPort
			action := ptp.FaultDetected

			logLine := fmt.Sprintf(
				"ptp4l 1361315.822 ptp4l.0.config 5 port 1 %s MASTER to %s on %s FT_UNSPECIFIED",
				ifname,
				strings.ToUpper(role.String()),
				strings.Replace(strings.ToUpper(action.String()), " ", "_", -1),
			)
			inChan <- logLine
			event := <-outChan
			values := event.Value()
			assert.Equal(t, event.SubType(), events.PortRole)
			assert.Equal(t, values[consts.InterfaceKey], ifname)
			assert.Equal(t, values[consts.ActionKey], action.String())
			assert.Equal(t, values[consts.RoleKey], role.String())
		},
	)

	t.Run(
		"listening to master port",
		func(t *testing.T) {

			ifname := "ens3f0"
			role := ptp.MasterPort
			preRole := ptp.ListeningPort
			action := ptp.AnnounceReceiptTimeoutExpires

			logLine := fmt.Sprintf(
				"ptp4l 1361315.822 ptp4l.0.config 5 port 1 %s %s to %s on %s ",
				ifname,
				strings.ToUpper(preRole.String()),
				strings.ToUpper(role.String()),
				strings.Replace(strings.ToUpper(action.String()), " ", "_", -1),
			)
			inChan <- logLine
			event := <-outChan
			values := event.Value()
			assert.Equal(t, event.SubType(), events.PortRole)
			assert.Equal(t, values[consts.InterfaceKey], ifname)
			assert.Equal(t, values[consts.ActionKey], action.String())
			assert.Equal(t, values[consts.RoleKey], role.String())
			assert.Equal(t, values[consts.PreviousRoleKey], preRole.String())
		},
	)

}
