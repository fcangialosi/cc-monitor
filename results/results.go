package results

import (
	"bytes"
	"encoding/gob"

	log "github.com/sirupsen/logrus"
)

type CCResults struct {
	Throughput map[string]([]map[float64]float64)
	Delay      map[string]map[float64]float64
	FlowTimes  map[string][]map[string]float64 // list of times when the flows "on" started
}

/* This struct is different because it contains the results for an individual algorithm*/
/* Used on upload to database*/
type DBResult struct {
	Alg        string
	Throughput []map[float64]float64
	Delay      map[float64]float64
	FlowTimes  []map[string]float64
}

/*Encodes cc result struct*/
func EncodeDBResult(res *DBResult) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(res.Throughput)
	e.Encode(res.Delay)
	e.Encode(res.FlowTimes)
	return w.Bytes()
}

func DecodeDBResult(data []byte) DBResult {
	results := DBResult{}
	r := bytes.NewBuffer(data)
	if data == nil || len(data) < 1 {
		log.Error("error decoding into Result struct")
	}
	d := gob.NewDecoder(r)
	d.Decode(&results.Throughput)
	d.Decode(&results.Delay)
	d.Decode(&results.FlowTimes)
	return results

}

/*Encodes the inner CC results struct*/
func EncodeCCResults(cc *CCResults) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(cc.Throughput)
	e.Encode(cc.Delay)
	e.Encode(cc.FlowTimes)
	return w.Bytes()
}

func DecodeCCResults(data []byte) CCResults {
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

/*Turn CCResult into DBResult*/
func BreakUpCCResult(cc *CCResults) map[string][]byte {
	result_map := make(map[string][]byte)
	for key, thr := range cc.Throughput {
		db_result := &DBResult{}
		db_result.Alg = key
		db_result.Throughput = thr
		db_result.Delay = cc.Delay[key]
		db_result.FlowTimes = cc.FlowTimes[key]
		result_map[key] = EncodeDBResult(db_result)
	}
	return result_map
}
