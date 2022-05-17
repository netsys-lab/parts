package main

import (
	"github.com/anacrolix/tagflag"
	parts "github.com/netsys-lab/parts/api"
	log "github.com/sirupsen/logrus"
)

var flags = struct {
	Server string
	Client string
	Mode   string
}{
	Server: "19-ffaa:1:cf1,[127.0.0.1]:8000",
	Client: "19-ffaa:1:cf1,[127.0.0.1]:8001",
	Mode:   "client", // "server" | "client" | "singlehost"
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
	log.Info("Starting singleconn")

	if flags.Mode == "server" {
		log.Infof("Running in server mode, listening on %s", flags.Server)
		conn, err := parts.Listen(flags.Server)
		if err != nil {
			log.Fatal(err)
		}

		n, buf, err := conn.ReadPart()
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Read part successfully, got %d bytes and buflen %d", n, len(buf))

	} else {
		buf := make([]byte, 2000)
		log.Infof("Finished loading. Dialing to server...")
		conn, err := parts.Dial(flags.Client, flags.Server)
		if err != nil {
			log.Fatal(err)
		}

		n, err := conn.Write(buf)
		if err != nil {
			log.Fatal(err)
		}

		log.Infof("Wrote part successfully, got %d bytes and buflen %d", n, len(buf))
	}
}
