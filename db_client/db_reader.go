package main
import (
	log "github.com/sirupsen/logrus"
  "../results"
  "os"
  "../config"
  "bufio"
  "sort"
  "fmt"
)

func checkErrMsg(err error, msg string) {
	if err != nil {
    log.WithFields(log.Fields{"msg": msg}).Panic(err)
	}
}

func readfile(filepath string, outfile string) {
  if _, err := os.Stat(filepath); os.IsNotExist(err) {
    log.Panic("Must provide a valid filepath")
  }

  filesizes := []uint32{1000,5000,10000}
  // open the file and read it
  f, err := os.Open(filepath)
  defer f.Close()
  checkErrMsg(err, "opening file")

  b := make([]byte, 50000000)
  // it is an encoded CCResults -> must decode it
  n, err := f.Read(b)
  checkErrMsg(err, "reading bytes into buf")
  results := results.DecodeCCResults(b[:n])

  outfile_fd, err := os.Create(outfile + ".csv")
  checkErrMsg(err, "opening csv file for writing")

  defer outfile_fd.Close()
  w := bufio.NewWriter(outfile_fd)
  // write header line into the file
  _, err = fmt.Fprintf(w, "delay,throughput,algorithm,filesize\n")
  for _, filesize := range filesizes {
    parseLogs(&results, filesize * 1000, outfile, w) // array is in KB, not bytes
  }
  w.Flush()

  fmt.Printf("Wrote data to %s\n", outfile + ".csv")
}

type ByUint32 []uint32
func (s ByUint32) Len() int {
  return len(s)
}

func (s ByUint32) Swap(i, j int) {
  s[i] , s[j] = s[j], s[i]
}

func (s ByUint32) Less(i,j int) bool {
  return s[i] < s[j]
}

type ByFloat32 []float32
func (s ByFloat32) Len() int {
  return len(s)
}

func (s ByFloat32) Swap(i, j int) {
  s[i] , s[j] = s[j], s[i]
}

func (s ByFloat32) Less(i,j int) bool {
  return s[i] < s[j]
}

func parseLogs(cc *results.CCResults, file_size uint32, outfile string, w *bufio.Writer) {

  /*for alg, flow_times := range cc.FlowTimes {
    log.WithFields(log.Fields{"algorithm": alg, "dict": flow_times}).Info("flow times")
  }*/


  for alg, thr := range cc.Throughput {
    log.WithFields(log.Fields{"alg": alg}).Info("result")
    if alg[:4] == "remy" {
      log.WithFields(log.Fields{"len of dictionary": len(thr), "dict": thr}).Info("ugh")
    }
    // right now - start with one flow -> get average throughput for everytime
    flow_tot_del := float32(0)
    flow_num_valid := 0
    flow_tot_thr := float32(0)

    for flow, flow_throughput := range thr {
       // average throughput in that flow
       bytes := make([]uint32, 0)
       for bytes_rec := range flow_throughput {
         bytes = append(bytes, bytes_rec)
       }
       ping_send_times := make([]float64, 0)
       for time_sent := range cc.Delay[alg] {
          ping_send_times = append(ping_send_times, float64(time_sent))
       }
       sort.Sort(ByUint32(bytes))
       sort.Float64s(ping_send_times)
       //log.WithFields(log.Fields{"ping_send_times": ping_send_times, "alg": alg}).Warn("pings in incr order")
       for _, bytes_rec := range bytes {
          if bytes_rec > file_size {
            // use this as the rough estimate of the throughput
            file_time := flow_throughput[bytes_rec]
            //log.WithFields(log.Fields{"bytes": bytes_rec, "time rec": file_time}).Info("byte count")
            thr_measurement := float32(bytes_rec) * float32(config.BYTES_TO_MBITS)/ (float32(file_time)/1000) // convert back to ms
            //log.WithFields(log.Fields{"file_size": file_size, "time": file_time, "flow": flow, "alg": alg}).Info("got to filesize")
            // get average delay until this time
            avg_delay := getAvgDelayUntil(cc.FlowTimes[alg][flow], cc.Delay[alg], file_time)
            log.WithFields(log.Fields{"file_size": file_size, "thr": thr_measurement, "avg_delay": avg_delay, "alg": alg, "flow": flow}).Info("Thr, delay pt")
          if avg_delay != 0 {
            flow_num_valid ++
            flow_tot_del += avg_delay
            flow_tot_thr += thr_measurement
          }
          break
       }
    }
  }
  flow_avg_del := float32(0)
  flow_avg_thr := float32(0)
  if flow_num_valid > 0 {
    flow_avg_del =  flow_tot_del/float32(flow_num_valid)
    flow_avg_thr = flow_tot_thr/float32(flow_num_valid)
    filesize_mb := file_size/1000000
    fmt.Fprintf(w, "%g,%g,%s,%dMB\n", flow_avg_del, flow_avg_thr, alg, filesize_mb)
  }

  //log.WithFields(log.Fields{"alg": alg, "thr_measurement": flow_avg_thr, "del_measurement": flow_avg_del, "file size": file_size}).Warn("ALG STATS")



}
}

func getAvgDelayUntil(flow_time_map map[string]float32, delay_map map[float32]float32, file_time float32) float32 {
  // first -> parse which delays are even within this "on" time
  /*for key, rtt := range delay_map {
  log.WithFields(log.Fields{"key": key, "rtt": rtt}).Info("del map")
  }*/
  flow_start := flow_time_map[config.START]
  flow_end := flow_time_map[config.END]
  file_arrival := flow_start + file_time

  relevant_rtts := make([]float32, 0)

  for send_time := range delay_map {
    if flow_start <= send_time && send_time <= flow_end {
      relevant_rtts = append(relevant_rtts, send_time)
    }
  }
  sort.Sort(ByFloat32(relevant_rtts))
  //log.WithFields(log.Fields{"list": relevant_rtts}).Info("relevant rtt list")
  if file_time > (flow_end - flow_start) {
    log.WithFields(log.Fields{"start": flow_start, "end": flow_end, "time": file_time}).Warn("Did not receive up to this file_time")
    return 0
  }
  //log.WithFields(log.Fields{"start": flow_start, "end": flow_end, "time": file_time, "ping before": (flow_start + file_time)}).Warn("Info")
  total_del := float32(0)
  num_del := 0
  for _, send_time := range relevant_rtts {
    if send_time <= file_arrival {
      total_del += delay_map[send_time]
      num_del++
    }
  }
  //log.WithFields(log.Fields{"total del": total_del}).Info("total delay")
  if num_del == 0 { return 0 }
  return total_del/float32(num_del) // average delay until that time in the flow
}

func main(){
  argsWithoutProg := os.Args[1:]
  if (len(argsWithoutProg) != 2) {
    log.Info("Please provide logs to read from, first argument, and outfile name to write to, second argument")
    os.Exit(0)
  }
  filepath := argsWithoutProg[0]
  outfile := argsWithoutProg[1] // csv data file for R script

  //log.Info(filepath)
  readfile(filepath, outfile)
}
