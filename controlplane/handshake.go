package controlplane

import (
	"bytes"
	"encoding/gob"

	"github.com/martenwallewein/parts/dataplane"
)

type PartsHandshake struct {
	Version                   uint8
	Flags                     uint16
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
	return &hs
}

func (hs *PartsHandshake) Decode() error {
	network := bytes.NewBuffer(hs.raw)
	dec := gob.NewDecoder(network)
	err := dec.Decode(hs)
	if err != nil {
		return err
	}
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
	hs.raw = network.Bytes()
	return nil
}
