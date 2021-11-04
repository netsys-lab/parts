package parts

import (
	"sync"

	"github.com/martenwallewein/parts/dataplane"
	"github.com/martenwallewein/parts/utils"
)

type MPOptions struct {
	NumConns int
}

type PartsSocket struct {
	local   string
	options *MPOptions
	conns   []*PartsConn
}

func newPartsSocket(local string, options *MPOptions) *PartsSocket {

	return &PartsSocket{
		local:   local,
		options: options, // TODO: handle default options
		conns:   make([]*PartsConn, 0),
	}
}

func ListenMP(local string, options *MPOptions) (*PartsSocket, error) {
	p := newPartsSocket(local, options)
	for i := 0; i < p.options.NumConns; i++ {
		conn, err := Listen(utils.IncreasePortInAddress(local, i+1)) // TODO: REUSE_PORT
		if err != nil {
			return nil, err
		}
		p.conns = append(p.conns, conn)
	}
	return p, nil
}

func DialMP(local, remote string, options *MPOptions) (*PartsSocket, error) {
	p := newPartsSocket(local, options)
	for i := 0; i < p.options.NumConns; i++ {
		conn, err := Dial(utils.IncreasePortInAddress(local, i+1), utils.IncreasePortInAddress(remote, i+1)) // TODO: REUSE_PORT
		if err != nil {
			return nil, err
		}
		p.conns = append(p.conns, conn)
	}
	return p, nil
}

func (p *PartsSocket) AcceptMP() error {
	// TODO: Move handshake only to first conn?
	// TODO: How to handle multiple errors?
	errChan := make(chan error, 0)
	for i := range p.conns {
		go func(i int) {
			conn := p.conns[i]
			err := conn.Accept()
			errChan <- err
		}(i)
	}
	for i := 0; i < len(p.conns); i++ {
		err := <-errChan
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PartsSocket) Write(b []byte) (n int, err error) {
	if len(b) <= dataplane.PACKET_PAYLOAD_SIZE {
		return p.conns[0].Write(b) // TODO: Scheduling?
	} else {
		conns := p.estimateConnsForData(b)
		bLen := len(b)
		var wg sync.WaitGroup
		partLen := utils.CeilForceInt(bLen, len(conns))
		for i := 0; i < len(conns); i++ {
			wg.Add(1)
			go func(index int, wg *sync.WaitGroup) {
				start := partLen * index
				end := utils.Min(start+partLen, bLen)
				conns[index].Write(b[start:end]) // PartIds positive TODO: Error handling
				wg.Done()
			}(i, &wg)
		}

		wg.Wait()
		return n, nil
	}
}

func (p *PartsSocket) Read(b []byte) (n int, err error) {
	if len(b) <= dataplane.PACKET_PAYLOAD_SIZE {
		return p.conns[0].Read(b) // TODO: Scheduling?
	} else {
		conns := p.estimateConnsForData(b)
		bLen := len(b)
		var wg sync.WaitGroup
		partLen := utils.CeilForceInt(bLen, len(conns))
		for i := 0; i < len(conns); i++ {
			wg.Add(1)
			go func(index int, wg *sync.WaitGroup) {
				start := partLen * index
				end := utils.Min(start+partLen, bLen)
				conns[index].Read(b[start:end]) // PartIds positive TODO: Error handling
				wg.Done()
			}(i, &wg)
		}

		wg.Wait()
		return n, nil
	}
}

// This one should do the magic...
func (p *PartsSocket) estimateConnsForData(b []byte) []*PartsConn {
	return p.conns // TODO: Real implementation
}
