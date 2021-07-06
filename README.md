# PARTS: Path-Aware Reliable Transport over SCION

## Implement a transport protocol
To add an own transport protocol, implement the interfaces `TransportSocket` and `TransportPacketPacker` from `socket/transportproto.go`.
In the examples you can add constructors that use you socket/packer implementations. See here:

```go
switch flags.Net {
	case "udp":
		socketConstructor = func() socket.TransportSocket { return socket.NewUDPTransportSocket() }
		packerConstructor = func() socket.TransportPacketPacker { return socket.NewUDPTransportPacketPacker() }
	}
```

The testtransport example can be used to perform in-memory transfer of a file to ensure
the implementations are working properly. This should help testing an own implementation,
so no two machines or network inbetween are required.
