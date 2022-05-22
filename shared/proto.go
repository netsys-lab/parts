package shared

const (
	PARTS_MSG_DATA   = 1 // Data packet
	PARTS_MSG_HS     = 2 // Handshake packet
	PARTS_MSG_ACK    = 3 // Data Acknowledgement packet
	PARTS_MSG_RT     = 4 // Retransfer packet
	PARTS_MSG_ACK_CT = 5 // Control Plane Acknowledgement packet
)

const (
	MASK_FLAGS_MSG     = 0b11111111
	MASK_FLAGS_VERSION = 0b11111111 << 16
)

const (
	PARTS_VERSION = 1 << 24
)

func NewPartsFlags() int64 {
	var flags int64 = 0
	flags = AddVersionFlag(PARTS_VERSION, MASK_FLAGS_VERSION)
	return flags
}

func AddMsgFlag(val, flag int64) int64 {
	return val | (flag & MASK_FLAGS_MSG)
}

func AddVersionFlag(val, flag int64) int64 {
	return val | (flag & MASK_FLAGS_VERSION)
}
