package testing

import "ptplogparser/pkg/process"

type MockProcess struct {
	ProcessName   string
	ResetCount    int
	ProcessStatus process.Status
}

func (m *MockProcess) Start() error {
	m.ProcessStatus = process.Running
	return nil
}

func (m *MockProcess) Stop() error {
	m.ProcessStatus = process.Stopped
	return nil
}

func (m *MockProcess) Name() string {
	return m.ProcessName
}

func (m *MockProcess) Reset() error {
	m.Stop()
	m.Start()
	m.ResetCount += 1
	return nil
}

func (m *MockProcess) Status() process.Status {
	return m.ProcessStatus
}
