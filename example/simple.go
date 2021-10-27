package main

import (
	"time"

	"github.com/anacrolix/tagflag"
	parts "github.com/martenwallewein/parts/api"
	log "github.com/sirupsen/logrus"
)

var flags = struct {
	Server      string
	Client      string
	ClientLocal string
	Mode        string
}{
	Server: "localhost:8000",
	Client: "localHost:8001",
	Mode:   "singlehost", // "server" | "client" | "singlehost"
}

func main() {
	tagflag.Parse(&flags)

	if flags.Mode == "singlehost" {
		conn, err := parts.Listen(flags.Server)
		if err != nil {
			log.Fatal(err)
		}

		conn2, err := parts.Dial(flags.Client, flags.Server)
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			time.Sleep(1 * time.Second)
			buf := make([]byte, 1000)
			n, err := conn2.Write(buf)
			if err != nil {
				log.Fatal(err)
			}
			log.Infof("Wrote %d bytes successfully", n)

		}()
		buf := make([]byte, 1000)
		n, err := conn.Read(buf)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Read %d bytes successfully", n)
	}
}
