package process

type Status int8

const (
	New Status = iota
	Running
	Dead
	Stopped
)

type EventType uint8

const (
	GNSSMetric EventType = iota
)

type Event interface {
	Marshal() ([]byte, error)
	Value() map[string]any
	SubType() EventType
}

type Process interface {
	Name() string
	Status() Status
	Start() error
	Stop() error
}
