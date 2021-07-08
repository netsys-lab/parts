// Downloads torrents from the command-line.
package main

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/martenwallewein/parts/socket"

	"github.com/anacrolix/tagflag"
	"github.com/martenwallewein/parts/api"
	log "github.com/sirupsen/logrus"
)

// var config *Config
var isServer bool

var flags = struct {
	IsServer   bool
	InFile     string
	OutFile    string
	NumCons    int
	Hash       string
	BufferSize int
	Net        string
	LocalAddr  string
	RemoteAddr string
	tagflag.StartPos
}{
	IsServer: true,
	NumCons:  1,
	Net:      "scion",
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

	isServer = flags.IsServer
	var file []byte
	var err error
	var buffer []byte
	if !isServer {
		file, err = ioutil.ReadFile(flags.InFile)
		Check(err)
		buffer = make([]byte, len(file))
	} else {
		buffer = make([]byte, flags.BufferSize)
	}

	var socketConstructor socket.TransportSocketConstructor
	var packerConstructor socket.TransportPackerConstructor

	switch flags.Net {
	case "udp":
		socketConstructor = func() socket.TransportSocket { return socket.NewUDPTransportSocket() }
		packerConstructor = func() socket.TransportPacketPacker { return socket.NewUDPTransportPacketPacker() }
	case "scion":
		socketConstructor = func() socket.TransportSocket { return socket.NewSCIONTransport() }
		packerConstructor = func() socket.TransportPacketPacker { return socket.NewSCIONPacketPacker() }
	}

	if isServer {
		// partSock := NewPartsSock("19-ffaa:1:c3f,[10.0.0.2]", "19-ffaa:1:cf0,[10.0.0.1]", 52000, 40000, 51000, 42000)
		partSock := api.NewPartsSock(
			flags.LocalAddr, flags.RemoteAddr,
			62000, 40000, 61000, 42000,
			flags.NumCons,
			socketConstructor,
			packerConstructor,
		)
		partSock.Listen()
		fmt.Println(len(buffer))
		log.Infof("Before receiving, buffer md5 %x", md5.Sum(buffer))
		// go partSock.ReadPart(buffer[:halfLen])
		// time.Sleep(10 * time.Millisecond)
		partSock.ReadPart(buffer)
		/*for i, v := range buffer {
			if v != file[i] {
				log.Infof("Byte index %d differs, value at buffer %b", i, v)
			}
		}*/
		log.Infof("Got %x md5 for received file compared to %s md5 for local", md5.Sum(buffer), flags.Hash)
		// err := ioutil.WriteFile(flags.OutFile, buffer, 777)
		// Check(err)
	} else {
		log.Infof("Client md5: %x", md5.Sum(file))
		log.Infof("Buffer len: %v", len(file))
		partSock := api.NewPartsSock(
			flags.LocalAddr, flags.RemoteAddr,
			40000, 62000, 42000, 61000,
			flags.NumCons,
			socketConstructor, packerConstructor,
		)
		partSock.SetMaxSpeed(1000000000)
		partSock.Dial()
		// go partSock.WritePart(file[:halfLen])
		// time.Sleep(10 * time.Millisecond)
		partSock.WritePart(file)
	}

	return nil
}
