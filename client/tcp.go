package main

import (
	"bytes"
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

func SetHeaderVal(b []byte, offset int, order binary.ByteOrder, val interface{}) *bytes.Buffer {
	p := bytes.NewBuffer(b[:offset])
	log.WithFields(log.Fields{"Receive time": val.(float64)}).Info("time being sent back")
	err := binary.Write(p, order, val.(float64))
	if err != nil {
		log.Fatal("binary.write failed")
	}
	return p
}
