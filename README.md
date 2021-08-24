# PARTS: Path-Aware Reliable Transport over SCION

**Note:** In the current state, PARTS only supports a fraction of the planned features and concepts, since it's under ongoing development. 

Parts is a reliable transport protocol based on UDP for high-performance multipath data transfer over SCION. The core concept of parts is splitting data into parts of variable size and transfer each part as a whole without requiring to keep packets in order. The partsize must be defined before sending the part to its receiver, but is not required to keep the same size for all parts. 

Parts aims to perform best in high-performance networks with less link congestions, since comes with aggressive rate control which assumes the receiver to support high bandwidth loads. According to many concepts in SDN, parts separates data and control plane. Using the control plane, a loose connection-oriented transfer is initiated using partIds that are places in the header of each packet. To begin a transfer of one or multiple parts, the sender and receiver perform a handshake about the parts that will be sent, configuration and system information. 

Instead of using a fixed buffer, parts adjusts its buffer for each part keeping all packets in memory until the transfer is complete.  To improve CPU cache usage, packets are prepared in consecutive memory layout and transferred from begin to end. Due to parts optimized sockets and probably less congested links, retransfers are assumed to occur with small probability and therefor are done at the end of each part. Since we observe limited performance for single UDP socket in Linux, parts introduce the concepts of **lines**. A line represents a socket on receiver or sender side. With the usage of **SO_REUSEPORT**, creating a socket does not necessarily means opening a network port, multiple sockets can also receive and send data behind the same port number. 

## Overview 
Figure 1 shows the data and control flow of transferring data over parts. The control plane runs on a separate socket to ensure it does not interfere with the data sockets. The data plane consists of 1 to n sockets that send and receive the actual data. Using SCIONs path-aware structure, each socket may use a dedicated path. However, also sharing paths between sockets is possible. The parts architecture consists of the following components.

![parts-structure (2)](https://user-images.githubusercontent.com/32448709/130609385-6b2da646-86f2-41a7-8cc6-69a22d40a131.jpg)


The **PartSocket** provides the API for applications to write and read parts to particular SCION endhosts. Furthermore, the PartSock initiates the transfer by sending a control packet to the receiver and waits for its response. This packet contains information about how many parts will be transferred, the number of available sockets to send data and further information regarding sending performance. After receiving the response, PartSocket passes the data buffer to the scheduler, which splits the data into parts and assigns them to available sockets. 

After completing the control phase, one or more parts are transferred sequentially or in parallel, depending on the number of available lines. The sender starts with initiating the sending phase of a part with creating part data packets in the respective buffer. This creation may be distributed over multiple threads to increase processing speed. Meanwhile, the sending thread assigned to the line writes the previously created packets on the network. All packets are sent in row before entering the next phase, which is responsible for possible retransfers.



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
