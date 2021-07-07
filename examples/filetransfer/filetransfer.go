// Downloads torrents from the command-line.
package main

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/anacrolix/tagflag"
	"github.com/martenwallewein/parts/api"
	log "github.com/sirupsen/logrus"
)

// var config *Config
var isServer bool

var flags = struct {
	IsServer   bool
	Config     string
	InFile     string
	OutFile    string
	NumCons    int
	Hash       string
	BufferSize int
	tagflag.StartPos
}{
	IsServer: true,
	Config:   "parts.toml",
	NumCons:  1,
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

	if isServer {
		// partSock := NewPartsSock("19-ffaa:1:c3f,[10.0.0.2]", "19-ffaa:1:cf0,[10.0.0.1]", 52000, 40000, 51000, 42000)
		partSock := api.NewPartsSock("127.0.0.1", "127.0.0.1", 2000, 40000, 51000, 42000, flags.NumCons)
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
		fmt.Println(len(buffer))
		partSock := api.NewPartsSock("127.0.0.1", "127.0.0.1", 40000, 52000, 42000, 51000, flags.NumCons)
		partSock.Dial()
		// go partSock.WritePart(file[:halfLen])
		// time.Sleep(10 * time.Millisecond)
		partSock.WritePart(file)
	}

	return nil
}
