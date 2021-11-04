package main

import (
	"time"

	"github.com/anacrolix/tagflag"
	parts "github.com/netsys-lab/parts/api"
	log "github.com/sirupsen/logrus"
)

var flags = struct {
	Server      string
	Client      string
	ClientLocal string
	Mode        string
}{
	Server: "19-ffaa:1:cf1,[127.0.0.1]:8000",
	Client: "19-ffaa:1:cf1,[127.0.0.1]:8001",
	Mode:   "singlehost", // "server" | "client" | "singlehost"
}

func main() {
	tagflag.Parse(&flags)

	if flags.Mode == "singlehost" {
		conn, err := parts.Listen(flags.Server)
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			time.Sleep(1 * time.Second)
			conn2, err := parts.Dial(flags.Client, flags.Server)
			if err != nil {
				log.Fatal(err)
			}
			buf := make([]byte, 2000000000)
			n, err := conn2.Write(buf)
			if err != nil {
				log.Fatal(err)
			}
			log.Infof("Wrote %d bytes successfully", n)

		}()
		err = conn.Accept()
		buf := make([]byte, 2000000000)
		n, err := conn.Read(buf)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Read %d bytes successfully", n)
	}
}
