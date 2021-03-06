package results

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type BytesTimeMap map[uint64]float32
type TimeRTTMap map[float32]float32
type OnOffMap map[string]float32

type IPList map[string]([]string)
type GraphInfo struct { // info about the server and time sent to get the correct graph URL
	ServerIP string
	SendTime string
}

type LossRTTInfo struct {
	LossRate float64
	Delay    TimeRTTMap
}
type CCResults struct {
	ServerIP   string
	ClientIP   string
	Throughput map[string]([]BytesTimeMap)
	Delay      map[string][]TimeRTTMap
	FlowTimes  map[string][]OnOffMap // list of times when the flows "on" started
	SendTime   string                // string of when client sent this result to the DB
}

func EncodeLossRTTInfo(info *LossRTTInfo) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(info.LossRate)
	e.Encode(info.Delay)
	return w.Bytes()
}

func DecodeLossRTTInfo(data []byte) LossRTTInfo {
	info := LossRTTInfo{}
	r := bytes.NewBuffer(data)
	if data == nil || len(data) < 1 {
		log.Error("Error decoding LossRTT info into struct")
		return info
	}
	d := gob.NewDecoder(r)
	d.Decode(&info.LossRate)
	d.Decode(&info.Delay)
	return info
}

/* This struct is different because it contains the results for an individual algorithm*/
/* Used on upload to database*/
type DBResult struct {
	ServerIP   string
	ClientIP   string
	Alg        string
	Throughput []BytesTimeMap
	Delay      []TimeRTTMap
	FlowTimes  []OnOffMap
}

func EncodeGraphInfo(gg *GraphInfo) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(gg.ServerIP)
	e.Encode(gg.SendTime)
	return w.Bytes()
}

func DecodeGraphInfo(data []byte) GraphInfo {
	results := GraphInfo{}
	r := bytes.NewBuffer(data)
	if data == nil || len(data) < 1 {
		log.Error("error decoding into GraphInfo struct")
	}
	d := gob.NewDecoder(r)
	d.Decode(&results.ServerIP)
	d.Decode(&results.SendTime)
	return results
}

func EncodeIPList(list IPList, order []string, num_cycles int) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(list)
	e.Encode(order)
	e.Encode(num_cycles)
	return w.Bytes()
}

func DecodeIPList(data []byte) (IPList, []string, int) {
	var res IPList
	var order []string
	var num_cycles int
	r := bytes.NewBuffer(data)
	if data == nil || len(data) < 1 {
		log.Error("error decoding into IP list")
	}
	d := gob.NewDecoder(r)
	d.Decode(&res)
	d.Decode(&order)
	d.Decode(&num_cycles)
	return res, order, num_cycles
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
	e.Encode(cc.SendTime)
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
	d.Decode(&results.SendTime)
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
	log.WithFields(log.Fields{"before": len(input), "after": len(buf.Bytes())})
	return buf.Bytes()

}

func checkError(err error) {
	if err != nil {
		log.Panic(err)
	}
}

// TODO move this to a separate package
func GetIPList(ip_file string) (IPList, []string, int) {
	ip_list := make(IPList)
	var ip_order []string
	var num_cycles int
	file, err := os.Open(ip_file)
	defer file.Close()
	checkError(err)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ip_line := scanner.Text() // line: IP UDP alg1 alg2 TCP alg1 alg2 ....
		if ip_line[:1] == "#" {
			continue
		}
		ip_line_split := strings.Split(ip_line, " ")
		log.Info(ip_line_split)
		if len(ip_line_split) == 1 {
			num_cycles, err = strconv.Atoi(ip_line_split[0])
			if err != nil {
				log.WithFields(log.Fields{"err": err}).Error("error reading num_cycles")
			}
			continue
		}
		ip := ip_line_split[0]
		algs := make([]string, 0)
		for _, val := range ip_line_split[2:] { // first val is IP, 2nd val is location
			algs = append(algs, val)
		}

		ip_list[ip] = algs
		ip_order = append(ip_order, ip)
		log.WithFields(log.Fields{"IP": ip, "algs": algs, "order": ip_order}).Info("Read from IP list")
	}
	checkError(scanner.Err())
	return ip_list, ip_order, num_cycles
}
