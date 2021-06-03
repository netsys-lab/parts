// Downloads torrents from the command-line.
package main

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/anacrolix/tagflag"
	log "github.com/sirupsen/logrus"
)

// var config *Config
var isServer bool

var flags = struct {
	IsServer bool
	Config   string
	InFile   string
	OutFile  string
	tagflag.StartPos
}{
	IsServer: true,
	Config:   "bengo.toml",
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

	file, err := ioutil.ReadFile(flags.InFile)
	Check(err)
	buffer := make([]byte, len(file))

	if isServer {
		blockSock := NewBlocksSock("127.0.0.1", "127.0.0.1", 30000, 40000, 31000, 42000)
		fmt.Println(len(buffer))
		log.Infof("Before receiving, buffer md5 %x", md5.Sum(buffer))
		blockSock.ReadBlock(buffer)
		/*for i, v := range buffer {
			if v != file[i] {
				log.Infof("Byte index %d differs, value at buffer %b", i, v)
			}
		}*/
		log.Infof("Got %x md5 for received file compared to %x md5 for local", md5.Sum(buffer), md5.Sum(file))
		// err := ioutil.WriteFile(flags.OutFile, buffer, 777)
		// Check(err)
	} else {
		fmt.Println(len(buffer))
		blockSock := NewBlocksSock("127.0.0.1", "127.0.0.1", 40000, 30000, 42000, 31000)
		blockSock.WriteBlock(file)
	}

	return nil
}
