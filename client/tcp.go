package main

import (
	"bytes"
	"encoding/binary"

	log "github.com/sirupsen/logrus"
)

func SetHeaderVal(b []byte, offset int, order binary.ByteOrder, val interface{}) *bytes.Buffer {
	p := bytes.NewBuffer(b[:offset])
	err := binary.Write(p, order, val.(float64))
	if err != nil {
		log.Fatal("binary.write failed")
	}
	return p
}

func ReadHeaderVal(b []byte, offset int, end_offset int, order binary.ByteOrder) int32 {
	var ret int32
	p := b[offset:end_offset] // end offset is not inclusive
	buf := bytes.NewReader(p)
	err := binary.Read(buf, binary.LittleEndian, &ret)
	if err != nil {
		log.Fatal("binary read failed")
	}
	//log.WithFields(log.Fields{"int": ret}).Info("read header val")
	return ret
}
