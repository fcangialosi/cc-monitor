package main

import (
	"bytes"
	"encoding/gob"

	log "github.com/sirupsen/logrus"
)

type CCResults struct {
	Throughput map[string]([]map[float64]float64)
	Delay      map[string]map[float64]float64
	FlowTimes  map[string][]float64 // list of times when the flows "on" started
}

/*Encodes the inner CC results struct*/
func (cc *CCResults) encode() []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(cc.Throughput)
	e.Encode(cc.Delay)
	e.Encode(cc.FlowTimes)
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
	d.Decode(&results.FlowTimes)
	return results
}
