package controlplane

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"

	"github.com/netsys-lab/parts/dataplane"
	"github.com/netsys-lab/parts/shared"
)

type PartsHandshake struct {
	Flags                     int64
	Reserved                  uint32
	MTU                       uint32
	InterfaceSpeed            uint64
	NumPorts                  uint32
	BufferSize                uint64
	EstimatedBandwidthPerPort uint64
	NumTransfers              uint32
	LocalAddr                 string // TODO: Fetch later from SCION header
	PartTransfers             []PartTransfer
	raw                       []byte
}

func NewPartsHandshake() *PartsHandshake {
	hs := PartsHandshake{
		raw: make([]byte, dataplane.PACKET_SIZE),
	}
	hs.prepareFlags()
	return &hs
}

func (hs *PartsHandshake) prepareFlags() {
	hs.Flags = shared.NewPartsFlags()
}

func (hs *PartsHandshake) Decode() error {
	flags := int64(binary.BigEndian.Uint64(hs.raw))
	network := bytes.NewBuffer(hs.raw[8:])
	dec := gob.NewDecoder(network)
	err := dec.Decode(hs)
	if err != nil {
		return err
	}
	hs.Flags = flags
	// TODO: Maybe we need this one, too
	// hs.raw = network.Bytes()
	return nil
}

func (hs *PartsHandshake) Encode() error {
	var network bytes.Buffer        // Stand-in for a network connection
	enc := gob.NewEncoder(&network) // Will write to network.
	err := enc.Encode(hs)
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint64(hs.raw, uint64(hs.Flags))
	copy(hs.raw[8:], network.Bytes())
	return nil
}
