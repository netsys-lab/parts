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

First SCION results:
```
1  Connections: Download took 7.257418898s with aggregated bandwidth 2453Mbit/s for 2097152000 bytes
2  Connections: Download took 4.780583741s with aggregated bandwidth 4292Mbit/s for 2097152000 bytes
3  Connections: Download took 3.622005011s with aggregated bandwidth 5718Mbit/s for 2097152000 bytes
4  Connections: Download took 3.723456199s with aggregated bandwidth 5716Mbit/s for 2097152000 bytes
5  Connections: Download took 3.924389940s with aggregated bandwidth 5720Mbit/s for 2097152000 bytes
6  Connections: Download took 3.910372057s with aggregated bandwidth 5712Mbit/s for 2097152000 bytes
7  Connections: Download took 4.030394961s with aggregated bandwidth 5508Mbit/s for 2097152000 bytes
8  Connections: Download took 3.935527850s with aggregated bandwidth 5712Mbit/s for 2097152000 bytes 
9  connections: Download took 4.203602862s with aggregated bandwidth 4600Mbit/s for 2097152000 bytes 
10 connections: Download took 4.343374496s with aggregated bandwidth 4280Mbit/s for 2097152000 bytes
```

Server:
```
ENABLE_FAST=true ./filetransfer -isServer=true -inFile="2000M.file" -outFile="2000M.file" -numCons=10 -hash=0cbf2bf1c1e5473ebfa262df2daf64d9 -bufferSize=2097152000 -localAddr='19-ffaa:1:c3f,[141.44.25.148]' -remoteAddr='19-ffaa:1:cf0,[141.44.25.151]'
```

Client:
```
ENABLE_FAST=true ./filetransfer -isServer=false -inFile="2000M.file" -outFile="2000M.file" -numCons=10 -remoteAddr='19-ffaa:1:c3f,[141.44.25.148]' -localAddr='19-ffaa:1:cf0,[141.44.25.151]'
```
