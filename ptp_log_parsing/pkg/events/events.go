package events

type EventType uint8

const (
	GNSSMetric EventType = iota
	PTPMeteric
	PortRole
	Phc2SysMetric
	Ts2phcMetric
)

type Event interface {
	Marshal() ([]byte, error)
	Value() map[string]any
	SubType() EventType
}
