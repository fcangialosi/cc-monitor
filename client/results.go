package main

import (
	"bytes"
	"encoding/gob"

	log "github.com/sirupsen/logrus"
)

type CCResults struct {
	Throughput map[string]map[float64]float64
	Delay      map[string]map[float64]float64
}

/*Encodes the inner CC results struct*/
func (cc *CCResults) encode() []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(cc.Throughput)
	e.Encode(cc.Delay)
	return w.Bytes()
}

func decodeCCResults(data []byte) CCResults {
	results := CCResults{}
	r := bytes.NewBuffer(data)
	if data == nil || len(data) < 1 {
		log.Error("error decoding into CCResults struct")
	}
	d := gob.NewDecoder(r)
	d.Decode(&results.Throughput)
	d.Decode(&results.Delay)
	return results
}
