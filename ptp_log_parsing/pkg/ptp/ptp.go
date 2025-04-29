package ptp

type State int8

const (
	Unknown State = iota
	FreeRun
	Locked
	Holdover

	FreeRunStr  = "FreeRun"
	LockedStr   = "Locked"
	HoldoverStr = "Holdover"
	UnknownStr  = "Unknown"
)

func (s State) String() string {
	switch s {
	case FreeRun:
		return FreeRunStr
	case Locked:
		return LockedStr
	case Holdover:
		return HoldoverStr
	default:
		return UnknownStr
	}
}

func StateFromPtp4l(s string) State {
	switch s {
	case "s0", "s1":
		return FreeRun
	case "s2", "s3":
		return Locked
	default:
		return Unknown
	}
}

type PortRole int8

const (
	PassivePort PortRole = iota
	FaultyPort
	ListeningPort
	MasterPort
	SlavePort
	UnknownPortRole // Some random default

	PassivePortStr   = "Passive"
	FaultyPortStr    = "Faulty"
	ListeningPortStr = "Listening"
	MasterPortStr    = "Master"
	SlavePortStr     = "Slave"
)

func (r PortRole) String() string {
	switch r {
	case PassivePort:
		return PassivePortStr
	case FaultyPort:
		return FaultyPortStr
	case ListeningPort:
		return ListeningPortStr
	case MasterPort:
		return MasterPortStr
	case SlavePort:
		return SlavePortStr
	case UnknownPortRole:
		return UnknownStr
	}
	return ""
}

func PortRoleFromPtp4l(s string) PortRole {
	switch s {
	case "FAULTY":
		return FaultyPort
	case "LISTENING":
		return ListeningPort
	case "MASTER":
		return MasterPort
	case "SLAVE":
		return SlavePort
	default:
		return UnknownPortRole // TODO: Decide what default should be
	}
}

type RoleAction int8

const (
	InitComplete RoleAction = iota
	AnnounceReceiptTimeoutExpires
	FaultDetected
	Other // Some random default

	InitCompleteStr                  = "Init Complete"
	AnnounceReceiptTimeoutExpiresStr = "Announce Receipt Timeout Expires"
	FaultDetectedStr                 = "Fault Detected"
	OtherStr                         = "Other"
)

func (r RoleAction) String() string {
	switch r {
	case InitComplete:
		return InitCompleteStr
	case AnnounceReceiptTimeoutExpires:
		return AnnounceReceiptTimeoutExpiresStr
	case FaultDetected:
		return FaultDetectedStr
	case Other:
		return OtherStr
	}
	return ""
}

func RoleActionFromPtp4l(s string) RoleAction {
	switch s {
	case "INIT_COMPLETE":
		return InitComplete
	case "ANNOUNCE_RECEIPT_TIMEOUT_EXPIRES":
		return AnnounceReceiptTimeoutExpires
	case "FAULT_DETECTED":
		return FaultDetected
	default:
		return Other // TODO: Decide what default should be
	}
}
