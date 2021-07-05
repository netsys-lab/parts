// Downloads torrents from the command-line.
package examples

import (
	"crypto/md5"
	"io/ioutil"
	"os"
	"sync"

	"github.com/anacrolix/tagflag"
	"github.com/martenwallewein/blocks/api"
	log "github.com/sirupsen/logrus"
)

var flags = struct {
	IsServer bool
	Config   string
	InFile   string
	OutFile  string
	tagflag.StartPos
}{}

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
	var wg sync.WaitGroup

	go func(wg *sync.WaitGroup) {
		wg.Add(1)
		// blockSock := NewBlocksSock("19-ffaa:1:c3f,[10.0.0.2]", "19-ffaa:1:cf0,[10.0.0.1]", 52000, 40000, 51000, 42000)
		blockSockServer := api.NewBlocksSock("127.0.0.1", "127.0.0.1", 52000, 40000, 51000, 42000)
		blockSockServer.Listen()
		log.Infof("Having buffer size of %d", len(buffer))
		log.Infof("Before receiving, buffer md5 %x", md5.Sum(buffer))
		blockSockServer.ReadBlock(buffer)
		log.Infof("Got %x md5 for received file compared to %x md5 for local", md5.Sum(buffer), md5.Sum(file))
		wg.Done()
	}(&wg)

	go func(wg *sync.WaitGroup) {
		wg.Add(1)
		blockSockClient := api.NewBlocksSock("127.0.0.1", "127.0.0.1", 40000, 52000, 42000, 51000)
		blockSockClient.Dial()
		blockSockClient.WriteBlock(file)
		wg.Done()
	}(&wg)

	wg.Wait()

	return nil
}
