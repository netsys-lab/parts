// Downloads torrents from the command-line.
package main

import (
	"crypto/md5"
	"io/ioutil"
	"os"
	"sync"

	"github.com/anacrolix/tagflag"
	"github.com/martenwallewein/parts/api"
	"github.com/martenwallewein/parts/socket"
	log "github.com/sirupsen/logrus"
)

var socketConstructor socket.TransportSocketConstructor
var packerConstructor socket.TransportPackerConstructor

var flags = struct {
	IsServer   bool
	Config     string
	InFile     string
	LocalAddr  string
	RemoteAddr string
	Net        string

	tagflag.StartPos
}{
	LocalAddr:  "127.0.0.1",
	RemoteAddr: "127.0.0.1",
	Net:        "udp",
}

func LogFatal(msg string, a ...interface{}) {
	log.Fatal(msg, a)
	os.Exit(1)
}

func Check(e error) {
	if e != nil {
		LogFatal("Fatal error. Exiting.", e, e.Error())
	}
}

func main() {
	if err := mainErr(); err != nil {
		log.Info("error in main: %v", err)
		os.Exit(1)
	}
}

func mainErr() error {
	tagflag.Parse(&flags)

	file, err := ioutil.ReadFile(flags.InFile)
	Check(err)
	buffer := make([]byte, len(file))

	log.Infof("net=%v", flags.Net)
	switch flags.Net {
	case "udp":
		socketConstructor = func() socket.TransportSocket { return socket.NewUDPTransportSocket() }
		packerConstructor = func() socket.TransportPacketPacker { return socket.NewUDPTransportPacketPacker() }
	case "scion":
		socketConstructor = func() socket.TransportSocket { return socket.NewSCIONTransport() }
		packerConstructor = func() socket.TransportPacketPacker { return socket.NewSCIONPacketPacker() }
	}

	var wg sync.WaitGroup
	log.Infof("Starting testtransport")
	log.Infof("Creating server")
	partSockServer := api.NewPartsSock(
		flags.LocalAddr, flags.RemoteAddr,
		52000, 40000, 51000, 42000,
		1,
		socketConstructor, packerConstructor,
	)

	partSockServer.Listen()
	partSockServer.EnableTestingMode()
	wg.Add(1)
	go func(wg *sync.WaitGroup) {

		// partSock := NewPartsSock("19-ffaa:1:c3f,[10.0.0.2]", "19-ffaa:1:cf0,[10.0.0.1]", 52000, 40000, 51000, 42000)

		log.Infof("Having buffer size of %d", len(buffer))
		log.Infof("Before receiving, buffer md5 %x", md5.Sum(buffer))
		partSockServer.ReadPart(buffer)
		log.Infof("Got %x md5 for received file compared to %x md5 for local", md5.Sum(buffer), md5.Sum(file))
		wg.Done()
	}(&wg)

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		log.Infof("Creating client")
		partSockClient := api.NewPartsSock(
			flags.RemoteAddr, flags.LocalAddr,
			40000, 52000, 42000, 51000,
			1,
			socketConstructor, packerConstructor,
		)
		partSockClient.EnableTestingMode()

		partSockClient.Dial()
		partSockClient.EnableTestingMode()
		partSockClient.WritePart(file)
		wg.Done()
	}(&wg)

	wg.Wait()

	return nil
}
