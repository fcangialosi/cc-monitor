package results

import (
	"bytes"
	"encoding/gob"
  "compress/gzip"
	log "github.com/sirupsen/logrus"
)
type IPList map[string](map[string][]string)
type CCResults struct {
	ServerIP   string
	ClientIP   string
	Throughput map[string]([]map[float64]float64)
	Delay      map[string]map[float64]float64
	FlowTimes  map[string][]map[string]float64 // list of times when the flows "on" started
}

/* This struct is different because it contains the results for an individual algorithm*/
/* Used on upload to database*/
type DBResult struct {
	ServerIP   string
	ClientIP   string
	Alg        string
	Throughput []map[float64]float64
	Delay      map[float64]float64
	FlowTimes  []map[string]float64
}

func EncodeIPList(list IPList) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(list)
	return w.Bytes()
}

func DecodeIPList(data []byte) IPList {
	var res IPList
	r := bytes.NewBuffer(data)
	if data == nil || len(data) < 1 {
		log.Error("error decoding into IP list")
	}
	d := gob.NewDecoder(r)
	d.Decode(&res)
	return res
}

/*Encodes cc result struct*/
func EncodeDBResult(res *DBResult) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(res.ServerIP)
	e.Encode(res.ClientIP)
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
	d.Decode(&results.ServerIP)
	d.Decode(&results.ClientIP)
	d.Decode(&results.Throughput)
	d.Decode(&results.Delay)
	d.Decode(&results.FlowTimes)
	return results

}

/*Encodes the inner CC results struct*/
func EncodeCCResults(cc *CCResults) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(cc.ServerIP)
	e.Encode(cc.ClientIP)
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
	d.Decode(&results.ServerIP)
	d.Decode(&results.ClientIP)
	d.Decode(&results.Throughput)
	d.Decode(&results.Delay)
	d.Decode(&results.FlowTimes)
	return results
}

/*Turn CCResult into DBResult*/
func BreakUpCCResult(cc *CCResults) map[string][]byte {
	result_map := make(map[string][]byte)
  for key, _ := range cc.Throughput {
    log.WithFields(log.Fields{"alg": key}).Info("DB RESULT")
  }
	for key, thr := range cc.Throughput {
    log.WithFields(log.Fields{"alg": key}).Info("DB RESULT")
		db_result := &DBResult{}
		db_result.Alg = key
		db_result.Throughput = thr
		db_result.Delay = cc.Delay[key]
		db_result.FlowTimes = cc.FlowTimes[key]
		db_result.ServerIP = cc.ServerIP
		db_result.ClientIP = cc.ClientIP
		result_map[key] = compress_array(EncodeDBResult(db_result))
	}
	return result_map
}
// TODO: add in decompress to get the results back
func compress_array(input []byte) []byte {
  var buf bytes.Buffer
  compr := gzip.NewWriter(&buf)
  compr.Write(input)
  compr.Close()
  log.WithFields(log.Fields{"before":len(input), "after": len(buf.Bytes())})
  return buf.Bytes()

}

