package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fcangialosi/cc-monitor/config"
	"github.com/fcangialosi/cc-monitor/results"
	"github.com/fcangialosi/cc-monitor/shared"

	//color "github.com/fatih/color"
	"github.com/fcangialosi/uiprogress"
	log "github.com/sirupsen/logrus"
)

var RETRY_LOCKED bool
var NAME string
var CLIENT_VERSION string
var BASE_PORT int

/*Simple function to print errors or ignore them*/
func CheckError(err error) bool {
	if err != nil {
		log.Error(err)
	}
	return err != nil
}

/*Gives elapsed time in milliseconds since a given start time*/
func elapsed(start time.Time) float32 {
	now := time.Now()
	dif := now.Sub(start).Seconds() * 1000
	//log.WithFields(log.Fields{"start": start, "now": now, "dif": dif}).Info("elapsed function")
	return float32(dif)
}

/*Used by both Remy and TCP*/
func singleThroughputMeasurement(t float32, bytes_received uint64) float32 {
	//log.WithFields(log.Fields{"t": t, "bytes received": bytes_received}).Warn("Time being passed into single throughout measurement function")
	return float32((float64(bytes_received) * config.BYTES_TO_MBITS) / (float64(t) / 1000)) // returns in Mbps
}

/*Measure throughput at increments*/
func measureThroughput(start time.Time, bytes_received uint64, m results.BytesTimeMap) {
	cur_time := elapsed(start)
	m[bytes_received] = cur_time
}

/*Record TCP throughput*/
func measureTCP(server_ip string, alg string, num_cycles int, cycle int, exp_time time.Duration, lock_servers bool, progress_string string) (results.BytesTimeMap, results.OnOffMap, results.TimeRTTMap, bool, bool) {
	flow_throughputs := results.BytesTimeMap{}
	flow_times := results.OnOffMap{}
	delay := results.TimeRTTMap{}
	flow_times[config.START] = float32(0)
	flow_times[config.END] = float32(0)

	original_start := time.Now()
	//start_ch <- original_start // start pings now
	//defer func() { end_ch <- time.Time{} }()

	// loop over each cycle and request TCP server for "1" on and off
	last_received_time := float32(0)
	recvBuf := make([]byte, config.TCP_TRANSFER_SIZE)
	bytes_received := uint64(0)

	// start connection
	gen_conn, err := net.DialTimeout("tcp", server_ip+":"+config.MEASURE_SERVER_PORT(BASE_PORT), config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "Failed to connect to server. Perhaps it is offline?") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, delay, true, false
	}

	var conn *net.TCPConn
	conn = gen_conn.(*net.TCPConn)

	defer conn.Close()

	// write timestamp for server to be able to identify this client connection later
	curTime := shared.UTCTimeString()
    origAlgName := alg
    algSp := strings.Split(origAlgName, " ")
    shortName := algSp[0] + shared.GetValues(origAlgName)

	if _, ok := shared.ParseAlgParams(alg)["exp_time"]; !ok {
		alg = alg + " exp_time=" + exp_time.String()
	}

	// write in the cycle number so the server knows which cycle this is for saving the TCPprobe and CCP logs file
	alg = fmt.Sprintf("%s trial=%d", alg, cycle+1)

	curr_exp, err := strconv.Atoi(strings.Split(progress_string, "/")[0])
	total_exp, err := strconv.Atoi(strings.Split(progress_string, "/")[1])
	if curr_exp == total_exp {
		lock_servers = false
	}
	total_time_second := (exp_time * time.Duration(total_exp)) / time.Second
	server_req := fmt.Sprintf("%s %t %s %s %d %s", curTime, lock_servers, NAME, CLIENT_VERSION, total_time_second, alg)
	conn.Write([]byte(server_req))
	// now wait for start
	n, err := conn.Read(recvBuf)
	resp := strings.SplitN(string(recvBuf[:n]), " ", 3)

	if resp[0] == config.SERVER_LOCKED {
		fmt.Printf("\rServer currently locked by %s for %s. ", resp[1], resp[2])
		if RETRY_LOCKED {
			fmt.Printf("I will continue to retry until I am able to connect.\n")
		} else {
			fmt.Printf("Skipping...\n")
		}
		return flow_throughputs, flow_times, delay, false, true
	}
	if resp[0] == config.VERSION_MISMATCH {
		fmt.Printf("\rERROR: Your client (%s) does not match the version of the server (%s). Please get an updated version of the client from: nimbus2000.csail.mit.edu", CLIENT_VERSION, resp[1])
		return flow_throughputs, flow_times, delay, false, false
	}
	if resp[0] == config.OTHER_ERROR {
		fmt.Printf("\rERROR: %s %s", resp[1], resp[2])
		return flow_throughputs, flow_times, delay, false, false
	}
	if resp[0] != config.START_FLOW {
		log.Error("Did not receive start from server")
		return flow_throughputs, flow_times, delay, true, false
	}
	clientPort := resp[1]
	//log.Info("client port: ", clientPort)

	// log.Info("Connection established.")
	fmt.Printf("Connection established.\n")

	// now start the timer
	start := time.Now() // start of flow is when client sends first message to send back data
	flow_times[config.START] = float32(start.Sub(original_start).Seconds() * 1000)
	conn.Write([]byte(config.ACK))
	if CheckErrMsg(err, "opening TCP connection") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, delay, true, false
	}

	started_flow := false
	//localPort := strings.Split(conn.LocalAddr().String(), ":")[1]

	// set first deadline for 30 seconds, then 30 seconds after
	dline := time.Now().Add(config.CLIENT_TIMEOUT * time.Second)
	conn.SetReadDeadline(dline)

	progress := uiprogress.New()
	progress.SetRefreshInterval(time.Millisecond * 500)
	var bar *uiprogress.Bar

    last := time.Now()
    curr_chunk_bytes := uint64(0)

    fmt.Printf("\nIP alg trial elapsed_ms chunk_bytes chunk_ms chunk_mbps\n")
	for {
		n, err := conn.Read(recvBuf)
		if err == io.EOF || n <= 0 {
			break
		} else if err, ok := err.(net.Error); ok && err.Timeout() {
			log.Warn("timeout")
			break
		} else if err != nil {
			log.Error(err)
		}

		if !started_flow {
			started_flow = true
			dline := time.Now().Add(exp_time)
			conn.SetReadDeadline(dline)
			fmt.Printf("\r")
			bar = progress.AddBar(dline, int(exp_time/time.Millisecond))
			bar.PrependSecRemaining()
			bar.PrependString(progress_string + " |")
			bar.AppendOtherBytes()
			//progress.Start()
		}

		bytes_received += uint64(n)
		curr_chunk_bytes += uint64(n)
		chunk_elapsed := time.Since(last) / 1e6
		last_received_time = elapsed(original_start)

        if chunk_elapsed >= 5000 {
            mbps := float32(curr_chunk_bytes) * 8.0 / (float32(chunk_elapsed) / 1000.0) / 1000000.0
            fmt.Printf("%s %s %d %f %d %d %f\n", server_ip, shortName, cycle+1, last_received_time, curr_chunk_bytes, chunk_elapsed, mbps)
            curr_chunk_bytes = 0
            last = time.Now()
        }

		//	bar.Set(int(last_received_time), int(bytes_received))
		measureThroughput(start, bytes_received, flow_throughputs)
	}
	//progress.Stop()
	fmt.Printf("Retrieving rtts from server...\n")
	conn2, err := net.DialTimeout("tcp", server_ip+":"+config.SRTT_INFO_PORT(BASE_PORT), config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "\rFailed to retrieve RTTs from server") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, delay, true, false
	}
	// else get the delay estimates from server
	infoBuf := make([]byte, 0)
	curtimePort := fmt.Sprintf("%s %s %s", curTime, clientPort, alg)
	conn2.Write([]byte(curtimePort))
	// read until the buf is full
	for {
		n, err = conn2.Read(recvBuf)
		if err == io.EOF {
			break
		}
		infoBuf = append(infoBuf, recvBuf[:n]...)
	}
	lossRTTInfo := results.DecodeLossRTTInfo(infoBuf)
	delay = lossRTTInfo.Delay

	// get avg delay
	fullDelay := float32(0)
	count := float32(0)
	for _, del := range delay {
		fullDelay += del
		count++
	}
	//BLUE := "\u001B[34m"
	//RED := "\u001B[31m"
	// ************ RESULTS ****
	//tput_mbps := color.BlueString(fmt.Sprintf("%0.1f", singleThroughputMeasurement(flow_throughputs[bytes_received], bytes_received)))
	tput_mbps := (fmt.Sprintf("%0.1f", singleThroughputMeasurement(flow_throughputs[bytes_received], bytes_received)))
	//delay_ms := color.RedString(fmt.Sprintf("%d", int(fullDelay/count)))
	delay_ms := (fmt.Sprintf("%d", int(fullDelay/count)))
	//proto := color.GreenString(shared.FriendlyAlgString(alg))
	//trial := color.MagentaString(fmt.Sprintf("%d", cycle+1))
	//algParams := shared.ParseAlgParams(alg)
	//expTime := ""
	//if val, ok := algParams["exp_time"]; ok {
	//	expTime = val
	//}
	//algParamsNoTime := shared.RemoveExpTime(alg)
	//output := fmt.Sprintf("\rproto:%s, trial:%s, tput_mbps: %s, delay_ms: %s, exptime_s: %s, alg_params: %s\n", proto, trial, tput_mbps, delay_ms, expTime, algParamsNoTime)

    fmt.Printf("\n%s %s %d %s %s\n\n", server_ip, shortName, cycle+1, tput_mbps, delay_ms)
	//fmt.Fprintf(os.Stdout, output)
	//fmt.Println("proto:%s,tput_mbps:%s%.1f,delay_ms:%s%d,elapsed:%.1f", alg, BLUE, tput_mbps, RED, delay_ms, elapsed)
	/*log.WithFields(log.Fields{
		"trial":                 cycle + 1,
		"bytes_received":        fmt.Sprintf("%.3f MBytes", float64(bytes_received)/1000000.0),
		"last_received_data_at": time.Duration(flow_throughputs[bytes_received]) * time.Millisecond,
		"time_elapsed":          elapsed(start) / 1000,
		"throughput":            fmt.Sprintf("%.3f Mbit/sec", singleThroughputMeasurement(flow_throughputs[bytes_received], bytes_received)),
		"avg_delay":             fmt.Sprintf("%.3f ms", fullDelay/count),
	}).Info("Finished Trial")*/

	flow_times[config.END] = last_received_time

	return flow_throughputs, flow_times, delay, false, false

}

func measureUDP(server_ip string, alg string, num_cycles int, cycle int, exp_time time.Duration, lock_servers bool, progress_string string) (results.BytesTimeMap, results.OnOffMap, results.TimeRTTMap, bool, bool) {
	timed_out := false
	flow_throughputs := results.BytesTimeMap{}
	flow_times := results.OnOffMap{}
	timeRTTMap := results.TimeRTTMap{}
	flow_times[config.START] = float32(0)
	flow_times[config.END] = float32(0)

	// send start to ping channel
	original_start := time.Now()
	// start_ch <- original_start
	// defer func() {
	// 	end_ch <- time.Time{}
	// }()

	// for each flow, start a separate connection to the server to spawn genericCC
	bytes_received := uint64(0)
	shouldEcho := (alg[:4] == "remy")
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)

	// create a TCP connection to get the genericCC port
	conn, err := net.DialTimeout("tcp", server_ip+":"+config.OPEN_UDP_PORT(BASE_PORT), config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "Open TCP connection to get genericCC port number") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, timeRTTMap, true, false
	}

	defer conn.Close()

	// create UDP listening port
	srcport := strconv.Itoa((9875 - cycle))
	conn.Write([]byte(alg + "->" + srcport)) // write remy
	//log.WithFields(log.Fields{"port": srcport}).Info("Listening on src port")

	n, err := conn.Read(recvBuf)
	if CheckErrMsg(err, "Trying to receive port number from genericCC") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, timeRTTMap, true, false
	}
	gccPort := string(recvBuf[:n])
	//log.WithFields(log.Fields{"port": gccPort}).Info("Received port number genericCC will be running on")

	laddr, err := net.ResolveUDPAddr("udp", ":"+srcport) // listen at a known port for later udp messages
	if CheckErrMsg(err, "creating laddr") {
		return flow_throughputs, flow_times, timeRTTMap, true, false
	}

	receiver, err := net.ListenUDP("udp", laddr)
	if CheckErrMsg(err, "error on creating receiver for listen UDP") {
		return flow_throughputs, flow_times, timeRTTMap, true, false
	}
	defer receiver.Close()

	// start listening for genericCC
	gccAddr, err := net.ResolveUDPAddr("udp", server_ip+":"+gccPort)
	if CheckErrMsg(err, "resolving addr to generic CC port given") {
		return flow_throughputs, flow_times, timeRTTMap, true, false
	}

	// have a go function now that listens on this for the loss rate and RTT Info from the connection
	lossRateChan := make(chan results.LossRTTInfo)
	go func() {
		// if for some reason - function returns in an error EARLY, conn will close and this goroutine will return because of EOF
		infoBuf := make([]byte, 0)
		for {
			bytes, errr := conn.Read(recvBuf)
			if errr != nil {
				// hopefully an EOF
				break
			}
			infoBuf = append(infoBuf, recvBuf[:bytes]...)
		}
		info := results.DecodeLossRTTInfo(infoBuf)
		lossRateChan <- info
		return
	}()

	start := time.Now()
	flow_start := float32(start.Sub(original_start).Seconds() * 1000)
	last_received_time := flow_start
	flow_times[config.START] = flow_start

	// loop of punching NAT and waiting for a responses
	attempts := 1
	for {
		receiver.WriteToUDP([]byte("open seasame"), gccAddr) // just send, ifnore any errors
		dline := time.Now().Add(config.HALF_MINUTE_TIMEOUT / 6 * time.Second)
		receiver.SetReadDeadline(dline)
		attempts++
		n, raddr, err := receiver.ReadFromUDP(recvBuf)
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				if attempts < config.MAX_CONNECT_ATTEMPTS {
					log.Warn("timeout, trying to initiate again...")
					continue
				} else {
					return flow_throughputs, flow_times, timeRTTMap, true, false
				}
			} else {
				log.Info("Error when punching NAT: ", err)
				return flow_throughputs, flow_times, timeRTTMap, true, false
			}

		}
		// reset start time
		start = time.Now()
		flow_start = float32(start.Sub(original_start).Seconds() * 1000)
		last_received_time = flow_start
		flow_times[config.START] = flow_start
		// send first echo and break
		if shouldEcho {
			echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, float64(elapsed(start)))
			receiver.WriteToUDP(echo.Bytes(), raddr)
		}

		break

	}

	log.Info("Hole punch successful. Connection established.")

	// initial timeout -> 30 Seconds
	dline := time.Now().Add(exp_time)
	receiver.SetReadDeadline(dline)

	for {
		n, raddr, err := receiver.ReadFromUDP(recvBuf)

		if err, ok := err.(net.Error); ok && err.Timeout() {
			// log.Warn("timeout")
			break
		} else if err == io.EOF {
			break
		} else if err != nil {
			log.Info("Non-nil err on sending UDP message: ", err)
			break
		}

		bytes_received += uint64(n)
		last_received_time = elapsed(original_start)
		measureThroughput(start, bytes_received, flow_throughputs)

		// echo packet with receive timestamp
		if shouldEcho {
			echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, float64(elapsed(start)))
			receiver.WriteToUDP(echo.Bytes(), raddr)
		}

	}

	// read from the buffer to get the lossRate Info
	lossRateInfo := <-lossRateChan
	lossRate := lossRateInfo.LossRate
	timeRTTMap = lossRateInfo.Delay
	delaySum := float32(0)
	count := float32(0)
	for _, rtt := range timeRTTMap {
		delaySum += rtt
		count++
	}
	log.WithFields(log.Fields{
		"trial":                 cycle + 1,
		"bytes_received":        fmt.Sprintf("%.3f MBytes", float64(bytes_received)/1000000.0),
		"last_received_data_at": time.Duration(flow_throughputs[bytes_received]) * time.Millisecond,
		"time_elapsed":          elapsed(start) / 1000,
		"throughput":            fmt.Sprintf("%.3f Mbit/sec", singleThroughputMeasurement(flow_throughputs[bytes_received], bytes_received)),
		"loss_rate":             lossRate,
		"avg_delay":             fmt.Sprintf("%.3f ms", delaySum/count),
	}).Info("Finished Trial")

	flow_times[config.END] = last_received_time
	return flow_throughputs, flow_times, timeRTTMap, timed_out, false
}

func runExperiment(f func(server_ip string, alg string, num_cycles int, cycle int, exp_time time.Duration, lock_servers bool, progress_string string) (results.BytesTimeMap, results.OnOffMap, results.TimeRTTMap, bool, bool), IP string, alg string, report *results.CCResults, protocol string, port string, num_cycles int, cycle int, exp_time time.Duration, lock_servers bool, progress_string string) (bool, bool) {
	throughput := make(results.BytesTimeMap)
	flow_times := make(results.OnOffMap)
	time_map := make(results.TimeRTTMap)
	timed_out := false
	locked := false

	throughput, flow_times, time_map, timed_out, locked = f(IP, alg, num_cycles, cycle, exp_time, lock_servers, progress_string)

	if !timed_out {
		report.Throughput[alg][cycle] = throughput
		report.FlowTimes[alg][cycle] = flow_times
		report.Delay[alg][cycle] = time_map
	}
	return timed_out, locked

}

func sendReport(report []byte) {
	// log.Info("Sending report to server")
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.DB_SERVER_PORT(), config.CONNECT_TIMEOUT*time.Second)
	if CheckError(err) {
		return
	}
	defer conn.Close()
	// write report in chunks
	chunkSize := 4000
	size := len(report)
	sent := 0
	for sent < size {
		if chunkSize >= (size - sent) {
			conn.Write(report[sent : sent+chunkSize])
			sent += chunkSize
		} else {
			conn.Write(report[sent:])
			sent = size
		}
	}
}

func getSendTimeLocalFile(localResultsStorage string) string { // if there is a localResults file, get the sendTime
	bytes, err := ioutil.ReadFile(localResultsStorage)
	if err != nil {
		log.Warn("err: ", err)
		return "NONE"
	}
	// decode the bytes
	tempReport := results.DecodeCCResults(bytes)
	return tempReport.SendTime
}

func runExperimentOnMachine(IP string, algs []string, num_cycles int, place int, total_experiments int, record bool, exp_time time.Duration, wait_time time.Duration, lock_servers bool) (string, int, int) {
	localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, IP)
	report := results.CCResults{
		ServerIP:   IP,
		ClientIP:   "NONE", // filled in later
		Throughput: make(map[string]([]results.BytesTimeMap)),
		Delay:      make(map[string][]results.TimeRTTMap),
		FlowTimes:  make(map[string][]results.OnOffMap),
		SendTime:   "NONE"}
	tempReport := results.CCResults{
		ServerIP:   "NONE",
		ClientIP:   "NONE", // filled in later
		Throughput: make(map[string]([]results.BytesTimeMap)),
		Delay:      make(map[string][]results.TimeRTTMap),
		FlowTimes:  make(map[string][]results.OnOffMap),
		SendTime:   "NONE"}

	var savedBytes []byte
	if _, err := os.Stat(localResultsStorage); err == nil {
		bytes, err := ioutil.ReadFile(localResultsStorage)
		if err == nil {
			savedBytes = bytes
		} else {
			log.Warn("err: ", err)
		}
		// decode the bytes
		tempReport = results.DecodeCCResults(savedBytes)
		// delete the file
		//err = os.Remove(localResultsStorage)
		if err != nil {
			log.Warn("Trying to remove local results file")
		}

	}

	// try to read the temporary results file - and check whether it is for this machine
	// also check if the record flag is turned on
	useTemp := (tempReport.ServerIP == IP && record)

	for _, alg_line := range algs {
		_, alg := shared.ParseAlg(alg_line)
		report.Throughput[alg] = make([]results.BytesTimeMap, num_cycles)
		report.Delay[alg] = make([]results.TimeRTTMap, num_cycles)
		report.FlowTimes[alg] = make([]results.OnOffMap, num_cycles)
	}

	// go through the temporary
	// have loop for cycles here
	cycle := 0
	this_exp_time := exp_time
	timed_out := false
	locked := true
	num_finished := 0
	retries := 0
	progress_string := ""
outer_loop:
	for cycle < num_cycles {
		// loop through each algorithm
		for _, alg_line := range algs {
			proto, alg := shared.ParseAlg(alg_line)

			// check if the result is in the report read at beginning of the function
			if useTemp && (len(tempReport.Throughput[alg][cycle]) > 0) && (len(tempReport.Delay[alg][cycle]) > 0) && (len(tempReport.FlowTimes[alg][cycle]) > 0) {
				report.Throughput[alg][cycle] = tempReport.Throughput[alg][cycle]
				report.Delay[alg][cycle] = tempReport.Delay[alg][cycle]
				report.FlowTimes[alg][cycle] = tempReport.FlowTimes[alg][cycle]
				// log.WithFields(log.Fields{"alg": alg, "cycle": cycle}).Warn("Used results in saved file for this algorithm and cycle")
				// log.Info("skipping")
				goto saveToFile // continue onto next algorithm and cycle
			} else {
				// log.WithFields(log.Fields{"alg": alg, "proto": proto, "server": IP}).Info(fmt.Sprintf("Starting Experiment %d of %d", place+1, total_experiments))
			}

			this_exp_time = exp_time
			if manual_exp_time, ok := shared.ParseAlgParams(alg)["exp_time"]; ok {
				new_exp_time, err := time.ParseDuration(manual_exp_time)
				if err != nil {
					log.Warn("unable to parse experiment time from params: ", manual_exp_time)
				} else {
					this_exp_time = new_exp_time
				}
			}

			timed_out = false
			locked = true
			retries = 0
			progress_string = fmt.Sprintf("%d/%d", place+1, total_experiments)
			for !timed_out && locked { // && retries < config.LOCKED_RETRIES {
				if proto == "tcp" {
					timed_out, locked = runExperiment(measureTCP, IP, alg, &report, "tcp", config.PING_TCP_SERVER_PORT(BASE_PORT), 1, cycle, this_exp_time, lock_servers, progress_string)
				} else if proto == "udp" {
					timed_out, locked = runExperiment(measureUDP, IP, alg, &report, "udp", config.PING_UDP_SERVER_PORT(BASE_PORT), 1, cycle, this_exp_time, lock_servers, progress_string)
				} else {
					log.Warn("Unknown protocol! Skipping...")
					break
				}
				if !locked {
					break
				}
				if !RETRY_LOCKED {
					break outer_loop
				}
				retries += 1
				time.Sleep(time.Second * config.RETRY_WAIT)
			}
			if !timed_out {
				num_finished += 1
			}
			time.Sleep(time.Second * 3)

			// write report so far into file
		saveToFile:
			b := results.EncodeCCResults(&report)
			err := ioutil.WriteFile(localResultsStorage, b, 0777)
			CheckErrMsg(err, "Writing file into bytes")
			place++
		}
		// after each round of algorithms, sleep for wait time
		if wait_time > 0 && cycle < num_cycles-1 {
			fmt.Printf("Waiting between trials...")
			time.Sleep(wait_time)
			clearStr := strings.Repeat(" ", len("Waiting between trials..."))
			fmt.Printf("\r%s", clearStr)
		}
		fmt.Printf("\n") // new line
		cycle++
	}

	sendTime := shared.UTCTimeString()
	report.SendTime = sendTime
	if num_finished > 0 {
		//sendReport(results.EncodeCCResults(&report))
	}
	// write the file - has a send time
	b := results.EncodeCCResults(&report)
	err := ioutil.WriteFile(localResultsStorage, b, 0777)
	CheckErrMsg(err, "Writing file into bytes")
	return sendTime, place, num_finished
}

func CheckErrMsg(err error, message string) bool { // check error
	if err != nil {
		log.WithFields(log.Fields{"msg": message}).Error(err)
	}
	return err != nil
}

func getURLFromServer(gg results.GraphInfo) string {
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.DB_GRAPH_PORT(), config.CONNECT_TIMEOUT*time.Second)
	if CheckError(err) {
		return "unknown"
	}
	defer conn.Close()
	conn.Write(results.EncodeGraphInfo(&gg))
	recvBuf := make([]byte, 2048)
	// now wait for the url
	n, _ := conn.Read(recvBuf)
	return string(recvBuf[:n])
}

func PullConfigFromServer() (shared.ServerList, int, time.Duration, time.Duration, bool, bool, int) {
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.IP_SERVER_PORT(), config.CONNECT_TIMEOUT*time.Second)
	if err != nil {
		log.Fatal("Error contacting config server: ", err)
	}
	defer conn.Close()
	recv_buf := make([]byte, config.LARGE_BUF_SIZE)

	n, err := conn.Read(recv_buf)
	if err != nil || n < 1 {
		log.Fatal("Error receiving config from server: ", err)
	}

	config := shared.DecodeConfig(recv_buf[:n])
	exp_time, err := time.ParseDuration(config.Exp_time)
	if err != nil {
		log.Fatal("Config contains invalid exp_time, expected format: [0-9]?(s|m|h)")
	}
	wait_time, err := time.ParseDuration(config.Wait_btwn_trial_time)
	if err != nil {
		log.Fatal("Config contains invalid wait_btwn_trial_time, expected format: [0-9]?(s|m|h)")
	}

	return config.Servers, config.Num_cycles, exp_time, wait_time, config.Lock_servers, config.Retry_locked, config.Pick_servers
}

//var use_mm = flag.Bool("mm", false, "If true, connect to a local server from inside a mahimahi shell")
//var manual_algs = flag.String("algs", "", "Specify a comma-separated list of algorithms to test, e.g. \"tcp-cubic,tcp-reno\"")
var local_iplist = flag.String("config", "", "Filename to read ips and algorithms from rather than pulling from server")
var should_resume = flag.Bool("resume", false, "Resume from most recent unfinished run")
var name = flag.String("name", "", "Nickname, easier to recognize than IP address, default: $USER")
var noupdate = flag.Bool("no-update", false, "Don't automatically update client if out of date")
var port = flag.Int("port", 10100, "Base port used to connect to all servers")

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if a == b {
			return true
		}
	}
	return false
}

func ensureClientUpToDate(my_version string, platform string) {
	if platform != "darwin" && platform != "linux" && platform != "windows" {
		log.Warn("Unknown platform ", platform)
		log.Warn("Skipping version check. Client may be out of date.")
		return
	}

	resp, err := http.Get("http://ccperf.csail.mit.edu/version")
	if err != nil {
		log.Warn(err)
		log.Warn("Failed to check version from ccperf.csail.mit.edu/version. Client may be out of date.")
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Warn(err)
		log.Warn("Failed to check version from ccperf.csail.mit.edu/version. Client may be out of date.")
		return
	}
	newest_version := strings.TrimSpace(string(body))

	if my_version != newest_version {
		fmt.Printf("Client (%s) is out of date.\nUpdating to newest version (%s)...\n", my_version, newest_version)

		// Get absolute path of current running executable
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			log.Warn(err)
			return
		}
		exs := strings.Split(os.Args[0], "/")
		ex := exs[len(exs)-1]
		currEx := fmt.Sprintf("%s/%s", dir, ex)

		// Download new executable to tmp file
		newEx := fmt.Sprintf("/tmp/ccperf-%s", newest_version)
		out, err := os.OpenFile(newEx, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			log.Warn(err)
			log.Warn("Failed to create tmp file for new client.")
			return
		}
		defer out.Close()

		resp, err := http.Get("http://ccperf.csail.mit.edu/ccperf-" + platform)
		if err != nil {
			log.Warn(err)
			log.Warn("Failed to get new client from server.")
			return
		}
		defer resp.Body.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			log.Warn(err)
			log.Warn("Failed to write new client executable.")
			return
		}

		// Move new executable to current executable location
		err = os.Rename(newEx, currEx)
		if err != nil {
			log.Warn(err)
			log.Warn("Failed to overwrite current client.")
			return
		}

		fmt.Println("Update complete.\n")

		// Switch to new executable
		if err := syscall.Exec(currEx, os.Args, os.Environ()); err != nil {
			log.Fatal(err)
		}

	}
}

/*Client will do Remy experiment first, then Cubic experiment, then send data back to the server*/
func main() {

	CLIENT_VERSION = "v2.3.8"
	fmt.Printf("ccperf client %s-%s\n\n", CLIENT_VERSION, runtime.GOOS)

	flag.Parse()
	BASE_PORT = *port

	if !*noupdate {
		ensureClientUpToDate(CLIENT_VERSION, runtime.GOOS)
	}

	finishedIPs := make([]string, 0)
	var progressFile *os.File
	if _, err := os.Stat(config.LOCAL_PROGRESS_FILE); *should_resume && err == nil {
		// read the IPs of this file into the finishedIPs - so we don't run those experiments
		progressFile, err = os.OpenFile(config.LOCAL_PROGRESS_FILE, os.O_RDWR, 0644)
		CheckErrMsg(err, "opening local progress file")
		scanner := bufio.NewScanner(progressFile)
		for scanner.Scan() {
			finishedIPs = append(finishedIPs, scanner.Text())
		}
	} else {
		progressFile, err = os.OpenFile(config.LOCAL_PROGRESS_FILE, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644) // also creates file
		CheckErrMsg(err, "Creating file to record local progress")
	}

	defer progressFile.Close()
	progressWriter := bufio.NewWriter(progressFile)

	// Default to just Remy and TCP Cubic

	/*
		if *use_mm {
			algs := []string{"tcp-cubic"}
			mahimahi := os.Getenv("MAHIMAHI_BASE")
			if *manual_algs != "" {
				algs = strings.Split(*manual_algs, ",")
			}
			num_cycles := 1
			runExperimentOnMachine(mahimahi, algs, num_cycles, 0, len(algs)*num_cycles, false, 30*time.Second, false)
		} else { // contact DB for files
	*/
	var servers shared.ServerList
	var num_cycles int
	var exp_time time.Duration
	var wait_time time.Duration
	var lock_servers bool
	var retry_locked bool
	var num_to_pick int
	if *local_iplist != "" {
		log.Info("Using local config")
		if _, err := os.Stat(*local_iplist); os.IsNotExist(err) {
			log.Fatal("Unable to find config file ", *local_iplist)
		}
		servers, num_cycles, exp_time, wait_time, lock_servers, retry_locked, num_to_pick = shared.ParseYAMLConfig(*local_iplist)
	} else {
		log.Info("Using remote config")
		servers, num_cycles, exp_time, wait_time, lock_servers, retry_locked, num_to_pick = PullConfigFromServer()
	}
	if num_to_pick <= 0 {
		num_to_pick = 1
	}
	if num_to_pick > len(servers) {
		num_to_pick = len(servers)
	}

	RETRY_LOCKED = retry_locked
	if *name != "" {
		NAME = *name
	} else {
		NAME = os.Getenv("USER")
	}
	if NAME == "" {
		NAME = "-"
	}
	// As per akshay's request: modify the servers so the client picks a new one everytime to run to
	// balls and bins problem now

	sendMap := make(map[string]string) // maps IPs to times the report was sent

	count := 1
	total_experiments := 0
	place := 0
	fmt.Printf("Found %d available test servers:\n", len(servers))
	for _, d := range servers {
		for ip, algs := range d {
			total_experiments += len(algs) * num_cycles
			if stringInSlice(ip, finishedIPs) {
				place += len(algs) * num_cycles
				count++
			}
		}
	}
	num_finished := 0
	num_servers_contacted := 0
	rand.Seed(time.Now().Unix())
	perm := rand.Perm(len(servers))
server_loop:
	for _, i := range perm {
		for ip, algs := range servers[i] {
			fmt.Printf("(%d)  %s\n", i+1, ip)
			sendTime := "NONE"
			place = 0
			new_place := 0
			if stringInSlice(ip, finishedIPs) {
				// file should be available to look for send time
				localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, ip)
				sendTime = getSendTimeLocalFile(localResultsStorage)
				sendMap[ip] = sendTime
				continue
			}
			fmt.Printf("\tAttempting to connect...")
			sendTime, new_place, num_finished = runExperimentOnMachine(ip, algs, num_cycles, place, len(algs), *should_resume, exp_time, wait_time, lock_servers)
			place = new_place
			sendMap[ip] = sendTime
			count++

			// the file should also be available, look for sendTime there
			localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, ip)
			sendTime = getSendTimeLocalFile(localResultsStorage)
			sendMap[ip] = sendTime
			fmt.Fprintf(progressWriter, "%s\n", ip)
			progressWriter.Flush()

			if num_finished >= num_to_pick {
				//if send_time, ok := sendMap[ip]; ok && send_time != "NONE" {
				//	info := results.GraphInfo{ServerIP: ip, SendTime: send_time}
				//	url := getURLFromServer(info)
				//	fmt.Printf("Results for server %s : %s\n", ip, url)
				//}
				num_servers_contacted += 1
				break server_loop
			}
		}

	}

	if num_servers_contacted <= 0 {
		fmt.Printf("\nAll servers are currently locked or unreachable.\nIf they are unreachable, please check your network connectivity.\nIf they are all locked, please try running the test again at a later time.\nThank you!\n")
	}

	// delete all the files
	//for _, d := range servers {
	//	for ip, _ := range d {
	//		localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, ip)
	//		err := os.Remove(localResultsStorage)
	//		if err != nil {
	//			log.Info("Error removing local results storage: ", localResultsStorage)
	//		}
	//	}
	//}

	// delete progress files
	err := os.Remove(config.LOCAL_PROGRESS_FILE)
	if err != nil {
		log.Info("Error removing local progress file: ", config.LOCAL_PROGRESS_FILE)
	}

	/*
		}
	*/
}
