package events

type EventType uint8

const (
	GNSSMetric EventType = iota
)

type Event interface {
	Marshal() ([]byte, error)
	Value() map[string]any
	SubType() EventType
}
