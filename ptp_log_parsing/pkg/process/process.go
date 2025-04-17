package process

type Status int8

const (
	New Status = iota
	Running
	Dead
	Stopped
)

type Process interface {
	Name() string
	Status() Status
	Start() error
	Stop() error
}
