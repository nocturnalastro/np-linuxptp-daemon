package event

import (
	"math"
	"testing"
	"time"

	"github.com/k8snetworkplumbingwg/linuxptp-daemon/pkg/utils"
)

func TestGetLargestOffset_MultiDPLL_FaultyPhaseOffset(t *testing.T) {
	t.Parallel()
	cfgName := "ptp4l.0.config"
	leadingIFace := "ens1f0"

	e := &EventHandler{
		data:         map[string][]*Data{},
		clkSyncState: map[string]*clockSyncState{},
	}

	e.clkSyncState[cfgName] = &clockSyncState{
		state:        PTP_LOCKED,
		leadingIFace: leadingIFace,
	}

	now := time.Now().UnixMilli()
	dpllWindow := utils.NewWindow(WindowSize)
	for i := 0; i < WindowSize; i++ {
		dpllWindow.Insert(50) // leading DPLL at 50ns offset
	}

	dpllData := &Data{
		ProcessName: DPLL,
		window:      *dpllWindow,
		Details: DDetails{
			{
				IFace:  leadingIFace,
				State:  PTP_LOCKED,
				Offset: 50,
				time:   now,
			},
			{
				IFace:  "ens2f0",
				State:  PTP_LOCKED,
				Offset: FaultyPhaseOffset, // follower DPLL out of range
				time:   now,
			},
		},
	}
	e.data[cfgName] = []*Data{dpllData}

	got := e.getLargestOffset(cfgName)

	// The FaultyPhaseOffset in the follower's dd.Offset should be the worst
	if got != FaultyPhaseOffset {
		t.Errorf("getLargestOffset() = %d, want %d (FaultyPhaseOffset from follower DPLL)", got, FaultyPhaseOffset)
	}
}

func TestGetLargestOffset_MultiDPLL_AllConverged(t *testing.T) {
	t.Parallel()
	cfgName := "ptp4l.0.config"
	leadingIFace := "ens1f0"

	e := &EventHandler{
		data:         map[string][]*Data{},
		clkSyncState: map[string]*clockSyncState{},
	}

	e.clkSyncState[cfgName] = &clockSyncState{
		state:        PTP_LOCKED,
		leadingIFace: leadingIFace,
	}

	now := time.Now().UnixMilli()
	dpllWindow := utils.NewWindow(WindowSize)
	for i := 0; i < WindowSize; i++ {
		dpllWindow.Insert(30) // leading DPLL stable at 30ns
	}

	dpllData := &Data{
		ProcessName: DPLL,
		window:      *dpllWindow,
		Details: DDetails{
			{
				IFace:  leadingIFace,
				State:  PTP_LOCKED,
				Offset: 30,
				time:   now,
			},
			{
				IFace:  "ens2f0",
				State:  PTP_LOCKED,
				Offset: -80, // follower at -80ns (within range but larger abs)
				time:   now,
			},
		},
	}
	e.data[cfgName] = []*Data{dpllData}

	got := e.getLargestOffset(cfgName)

	// Leading uses window.Mean()=30, follower uses dd.Offset=-80
	// Worst = max(abs(30), abs(-80)) = -80
	if math.Abs(float64(got)) != 80 {
		t.Errorf("getLargestOffset() = %d, want ±80 (follower DPLL offset)", got)
	}
}

func TestGetLargestOffset_EmptyWindow_Skipped(t *testing.T) {
	t.Parallel()
	cfgName := "ptp4l.0.config"
	leadingIFace := "ens1f0"

	e := &EventHandler{
		data:         map[string][]*Data{},
		clkSyncState: map[string]*clockSyncState{},
	}

	e.clkSyncState[cfgName] = &clockSyncState{
		state:        PTP_LOCKED,
		leadingIFace: leadingIFace,
	}

	// PTP4l data with empty window (no offsets sent yet)
	ptp4lData := &Data{
		ProcessName: PTP4l,
		window:      *utils.NewWindow(WindowSize),
	}

	now := time.Now().UnixMilli()
	dpllWindow := utils.NewWindow(WindowSize)
	for i := 0; i < WindowSize; i++ {
		dpllWindow.Insert(25)
	}
	dpllData := &Data{
		ProcessName: DPLL,
		window:      *dpllWindow,
		Details: DDetails{
			{
				IFace:  leadingIFace,
				State:  PTP_LOCKED,
				Offset: 25,
				time:   now,
			},
		},
	}

	e.data[cfgName] = []*Data{ptp4lData, dpllData}

	got := e.getLargestOffset(cfgName)

	// PTP4l window is empty → skipped, DPLL window mean = 25
	if got != 25 {
		t.Errorf("getLargestOffset() = %d, want 25 (DPLL window mean, PTP4l skipped)", got)
	}
}

func TestAddEvent_FaultyPhaseOffset_NotInsertedInWindow(t *testing.T) {
	t.Parallel()
	d := &Data{
		ProcessName: DPLL,
		window:      *utils.NewWindow(WindowSize),
		Details: DDetails{
			{
				IFace:  "ens1f0",
				State:  PTP_LOCKED,
				Offset: 0,
				time:   0,
			},
		},
	}

	// Insert a valid offset first
	d.AddEvent(EventChannel{
		ProcessName: DPLL,
		IFace:       "ens1f0",
		State:       PTP_LOCKED,
		Time:        time.Now().UnixMilli(),
		Values:      map[ValueType]interface{}{OFFSET: int64(50)},
	})

	if d.window.IsEmpty() {
		t.Error("window should not be empty after valid offset insert")
	}

	// Insert FaultyPhaseOffset
	d.AddEvent(EventChannel{
		ProcessName: DPLL,
		IFace:       "ens1f0",
		State:       PTP_LOCKED,
		Time:        time.Now().UnixMilli() + 1,
		Values:      map[ValueType]interface{}{OFFSET: FaultyPhaseOffset},
	})

	// dd.Offset should have the sentinel
	if d.Details[0].Offset != FaultyPhaseOffset {
		t.Errorf("dd.Offset = %d, want FaultyPhaseOffset", d.Details[0].Offset)
	}

	// Window mean should still be 50 (sentinel was not inserted)
	got := d.window.Mean()
	if got != 50.0 {
		t.Errorf("window.Mean() = %f, want 50.0 (sentinel should not be in window)", got)
	}
}
