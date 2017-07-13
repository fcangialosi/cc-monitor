package main

import (
	"bytes"
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

func SetHeaderVal(b []byte, offset int, order binary.ByteOrder, val interface{}) *bytes.Buffer {
	p := bytes.NewBuffer(b)
	p.Next(offset)
	err := binary.Write(p, order, val)
	if err != nil {
		log.Fatal("binary.write failed")
	}
	return p
}
