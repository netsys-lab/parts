package control

import (
	"fmt"
	"net"

	"github.com/martenwallewein/blocks/socket"
)

type ControlPeer struct {
	remoteCtrlPort int
	remoteCtrlAddr *net.UDPAddr
}

type ControlPlane struct {
	localCtrlPort int
	localCtrlAddr string
	ControlSocket socket.TransportSocket
	Peers         []ControlPeer
}

func NewControlPlane(localCtrlPort int, remoteCtrlPort int, remoteCtrlAddr string, localCtrlAddr string) (*ControlPlane, error) {
	// Make that thing Transport safe
	cp := ControlPlane{
		localCtrlPort: localCtrlPort,
		localCtrlAddr: localCtrlAddr,
		Peers:         make([]ControlPeer, 0),
	}
	sock := socket.NewUDPTransportSocket()
	err := sock.Listen(fmt.Sprintf("%s:%d", localCtrlAddr, localCtrlPort))
	if err != nil {
		return nil, err
	}
	err = sock.Dial(fmt.Sprintf("%s:%d", remoteCtrlAddr, remoteCtrlPort))
	if err != nil {
		return nil, err
	}

	cp.ControlSocket = sock
	return &cp, nil
}

// TBD: For Bittorrent later, maybe share control plane between socket instances
func (cp *ControlPlane) AddControlPeer(remoteCtrlPort int, remoteCtrlAddr *net.UDPAddr) {
	cp.Peers = append(cp.Peers, ControlPeer{remoteCtrlPort: remoteCtrlPort, remoteCtrlAddr: remoteCtrlAddr})
}

// TBD: Extract to ReadRetransfers, etc
func (cp *ControlPlane) Read(buf []byte) (int, error) {
	return cp.ControlSocket.Read(buf)
}

// TBD: Extract to WriteRetransfers, etc
func (cp *ControlPlane) Write(buf []byte) (int, error) {
	return cp.ControlSocket.Read(buf)
}
