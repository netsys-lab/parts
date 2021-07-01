package control

import (
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
	ControlSocket *socket.SCIONSocket
	Peers         []ControlPeer
}

func NewControlPlane(localCtrlPort int, remoteCtrlPort int, remoteCtrlAddr string, localCtrlAddr string) (*ControlPlane, error) {
	// Make that thing Transport safe
	cp := ControlPlane{
		localCtrlPort: localCtrlPort,
		localCtrlAddr: localCtrlAddr,
		Peers:         make([]ControlPeer, 0),
	}

	sock, err := socket.NewSCIONSocket(localCtrlAddr, localCtrlPort)
	if err != nil {
		return nil, err
	}
	cp.ControlSocket = sock
	return &cp, nil
}

func (cp *ControlPlane) AddControlPeer(remoteCtrlPort int, remoteCtrlAddr *net.UDPAddr) {
	cp.Peers = append(cp.Peers, ControlPeer{remoteCtrlPort: remoteCtrlPort, remoteCtrlAddr: remoteCtrlAddr})
}
