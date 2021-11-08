package main

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"io/ioutil"
	"time"

	"github.com/anacrolix/tagflag"
	parts "github.com/netsys-lab/parts/api"
	log "github.com/sirupsen/logrus"
)

var flags = struct {
	Server  string
	Client  string
	Mode    string
	NumCons int
	Path    string
}{
	Server:  "19-ffaa:1:cf1,[127.0.0.1]:8000",
	Client:  "19-ffaa:1:cf1,[127.0.0.1]:8001",
	Mode:    "singlehost", // "server" | "client" | "singlehost"
	NumCons: 1,
}

type MetaPacket struct {
	FileSize int
	Md5      [16]byte
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})
	log.SetLevel(log.DebugLevel)
	tagflag.Parse(&flags)
	log.Info(flags)
	log.Info("Starting filetransfer")

	if flags.Mode == "server" {
		log.Infof("Running in server mode, listening on %s", flags.Server)
		socket, err := parts.ListenMP(flags.Server, &parts.MPOptions{
			NumConns: flags.NumCons,
		})
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Accepting client...")
		err = socket.AcceptMP()
		if err != nil {
			log.Fatal(err)
		}

		log.Infof("Accepting file handshake...")
		metaPacket, err := incomingHandshake(socket)
		if err != nil {
			log.Fatal(err)
		}

		log.Infof("Reading file...")
		file := make([]byte, metaPacket.FileSize)
		_, err = socket.Read(file)
		if err != nil {
			log.Fatal(err)
		}

		log.Infof("Read successful %d bytes, got md5 %x for expected md5 %x", len(file), md5.Sum(file), metaPacket.Md5)

	} else {
		log.Infof("Running in client mode, listening on %s and dialing to %s", flags.Client, flags.Server)
		log.Infof("Loading file %s to RAM", flags.Path)
		file, err := ioutil.ReadFile(flags.Path)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Finished loading. Dialing to server...")
		socket, err := parts.DialMP(flags.Client, flags.Server, &parts.MPOptions{
			NumConns: flags.NumCons,
		})
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Starting file handshake...")
		metaPacket, err := outgoingHandshake(socket, file)
		if err != nil {
			log.Fatal(err)
		}

		log.Infof("Starting file transfer...")
		time.Sleep(1 * time.Second)
		_, err = socket.Write(file)
		if err != nil {
			log.Fatal(err)
		}

		log.Infof("Write successful for %d bytes and md5 %x ", len(file), metaPacket.Md5)
	}
}

func outgoingHandshake(socket *parts.PartsSocket, file []byte) (*MetaPacket, error) {
	metaPacket := MetaPacket{
		FileSize: len(file),
		Md5:      md5.Sum(file),
	}
	log.Debugf("GOt md5")
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(metaPacket)
	if err != nil {
		return nil, err
	}
	_, err = socket.Write(network.Bytes())
	if err != nil {
		return nil, err
	}

	log.Debugf("Wrote packet")

	buf := make([]byte, 1000)
	_, err = socket.Read(buf)
	if err != nil {
		return nil, err
	}
	log.Debugf("FInished outgoinf")
	return &metaPacket, nil
}

func incomingHandshake(socket *parts.PartsSocket) (*MetaPacket, error) {
	buf := make([]byte, 1000)
	bts, err := socket.Read(buf)
	if err != nil {
		return nil, err
	}
	network := bytes.NewBuffer(buf[:bts])
	dec := gob.NewDecoder(network)
	metaPacket := MetaPacket{}
	err = dec.Decode(&metaPacket)
	if err != nil {
		return nil, err
	}
	var n bytes.Buffer
	enc := gob.NewEncoder(&n)
	err = enc.Encode(metaPacket)
	if err != nil {
		return nil, err
	}
	_, err = socket.Write(n.Bytes())
	return &metaPacket, nil
}
