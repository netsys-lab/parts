# PARTS: Path-Aware Reliable Transport over SCION

**Note:** In the current state, PARTS is in alpha stage and only supports a fraction of the planned features and concepts, since it's under ongoing development. You may observe crashes, freezes, unexpected behavior or flipped bits during transfer. 

Parts is a reliable transport protocol based on UDP for high-performance multipath data transfer over SCION. The core concept of parts is splitting data into parts of variable size and transfer each part as a whole without requiring to keep packets in order. The partsize must be defined before sending the part to its receiver, but is not required to keep the same size for all parts. 

Parts aims to perform best in high-performance networks with less link congestions, since comes with aggressive rate control which assumes the receiver to support high bandwidth loads. According to many concepts in SDN, parts separates data and control plane. Using the control plane, a loose connection-oriented transfer is initiated using partIds that are places in the header of each packet. To begin a transfer of one or multiple parts, the sender and receiver perform a handshake about the parts that will be sent, configuration and system information. 

Instead of using a fixed buffer, parts adjusts its buffer for each part keeping all packets in memory until the transfer is complete.  To improve CPU cache usage, packets are prepared in consecutive memory layout and transferred from begin to end. Due to parts optimized sockets and probably less congested links, retransfers are assumed to occur with small probability and therefor are done at the end of each part. Since we observe limited performance for single UDP socket in Linux, parts introduce the concepts of **lines**. A line represents a socket on receiver or sender side that transfer data to each other. 

## Usage

### Single connection

```go
// Listen
conn, err := parts.Listen(flags.Server)
if err != nil {
    log.Fatal(err)
}
err = conn.Accept() // Blocks until a Dial arrives

// Dial, Client = Local Address, Server = Remote Address
conn2, err := parts.Dial(flags.Client, flags.Server) 
if err != nil {
    log.Fatal(err)
}

// net.Conn interface implemented
buf := make([]byte, 20000)

// Important: PARTS works best passing oversized buffers to read/write
// giving possbility to optimize transfer
// Both calls block until the data is transferred completely
n, err := conn.Read(buf)
n, err := conn2.Write(buf)
```

### Multipath Connection (under construction...)

```go
// Listen
socket, err := parts.ListenMP(flags.Server, &parts.MPOptions{
    NumConns: flags.NumCons,
})
if err != nil {
    log.Fatal(err)
}

// Again blocking...
err = socket.AcceptMP()

// Dial
socket2, err := parts.DialMP(flags.Client, flags.Server, &parts.MPOptions{
    NumConns: flags.NumCons,
})
if err != nil {
    log.Fatal(err)
}

// Again do not split buffers into packet-sized buffers before transferring them, transfer the whole buffer, parts does the rest
buf := make([]byte, 20000)

// Not complete net.Conn interface implemented, may come...
n, err := socket.Read(buf)
n, err := socket2.Write(buf)

```
