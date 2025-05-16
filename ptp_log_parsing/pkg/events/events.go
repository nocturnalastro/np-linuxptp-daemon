package events

type EventType uint8

const (
	GNSSMetric EventType = iota
	PTPMeteric
	PortRole
	Ptp4lOffsetEvent
	Phc2SysMetric
	Ts2phcOffset
)

type Event interface {
	Marshal() ([]byte, error)
	Value() map[string]any
	SubType() EventType
}
