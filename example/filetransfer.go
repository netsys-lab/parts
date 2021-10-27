package main

import (
	"log"

	parts "github.com/martenwallewein/parts/api"
)

func main() {

	conn, err := parts.Listen("addr1")
	if err != nil {
		log.Fatal(err)
	}

	conn2, err := parts.Dial("addr2", "addr1")
	if err != nil {
		log.Fatal(err)
	}

	// Read file info packet
	fileInfo := make([]byte, 1200)
	conn.Read(fileInfo)
	size := 21398384
	data := make([]byte, size)
	conn.Read(data)
}
