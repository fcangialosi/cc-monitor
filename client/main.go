package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"../config"
	"../results"
	"../shared"
	log "github.com/sirupsen/logrus"
)

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
func singleThroughputMeasurement(t float32, bytes_received uint32) float32 {
	//log.WithFields(log.Fields{"t": t, "bytes received": bytes_received}).Warn("Time being passed into single throughout measurement function")
	return (float32(bytes_received) * config.BYTES_TO_MBITS) / (t / 1000) // returns in Mbps
}

/*Measure throughput at increments*/
func measureThroughput(start time.Time, bytes_received uint32, m results.BytesTimeMap) {
	cur_time := elapsed(start)
	m[bytes_received] = cur_time
}

/*Record TCP throughput*/
func measureTCP(server_ip string, alg string, num_cycles int, cycle int, exp_time time.Duration) (results.BytesTimeMap, results.OnOffMap, results.TimeRTTMap, bool) {
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
	bytes_received := uint32(0)

	// start connection
	conn, err := net.DialTimeout("tcp", server_ip+":"+config.MEASURE_SERVER_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "tcp connection to server") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, delay, true
	}
	// write timestamp for server to be able to identify this client connection later
	curTime := currentTime()
	if _, ok := shared.ParseAlgParams(alg)["exp_time"]; !ok {
		alg = alg + " exp_time=" + exp_time.String()
	}
	algTime := fmt.Sprintf("%s %s", curTime, alg)
	conn.Write([]byte(algTime))
	// now wait for start
	n, err := conn.Read(recvBuf)
	if string(recvBuf[:n]) != config.START_FLOW {
		log.Error("Did not receive start from server")
	}
	// now start the timer
	start := time.Now() // start of flow is when client sends first message to send back data
	flow_times[config.START] = float32(start.Sub(original_start).Seconds() * 1000)
	conn.Write([]byte(config.ACK))
	if CheckErrMsg(err, "opening TCP connection") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, delay, true
	}

	// set first deadline for 30 seconds, then 30 seconds after
	started_flow := false
	dline := time.Now().Add(config.CLIENT_TIMEOUT * time.Second)
	conn.SetReadDeadline(dline)
	localPort := strings.Split(conn.LocalAddr().String(), ":")[1]
	// log.WithFields(log.Fields{"deadline": dline}).Info("set read deadline")

	for {
		//log.Info("Waiting to read")

		n, err := conn.Read(recvBuf)
		//log.Info("read")

		if err == io.EOF || n <= 0 {
			//log.Warn("Server closed connection")
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
		}

		bytes_received += uint32(n)
		last_received_time = elapsed(original_start)
		measureThroughput(start, bytes_received, flow_throughputs)

	}
	conn2, err := net.DialTimeout("tcp", server_ip+":"+config.SRTT_INFO_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "tcp connection to server") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, delay, true
	}
	// else get the delay estimates from server
	infoBuf := make([]byte, 0)
	curtimePort := fmt.Sprintf("%s->%s", curTime, localPort)
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
	log.WithFields(log.Fields{
		"trial":                 cycle + 1,
		"bytes_received":        fmt.Sprintf("%.3f MBytes", float64(bytes_received)/1000000.0),
		"last_received_data_at": time.Duration(flow_throughputs[bytes_received]) * time.Millisecond,
		"time_elapsed":          elapsed(start) / 1000,
		"throughput":            fmt.Sprintf("%.3f Mbit/sec", singleThroughputMeasurement(flow_throughputs[bytes_received], bytes_received)),
		"avg_delay":             fmt.Sprintf("%.3f ms", fullDelay/count),
	}).Info("Finished Trial")

	flow_times[config.END] = last_received_time
	conn.Close() // close connection before next one

	return flow_throughputs, flow_times, delay, false

}

func measureUDP(server_ip string, alg string, num_cycles int, cycle int, exp_time time.Duration) (results.BytesTimeMap, results.OnOffMap, results.TimeRTTMap, bool) {
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
	bytes_received := uint32(0)
	shouldEcho := (alg[:4] == "remy")
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)

	// create a TCP connection to get the genericCC port
	conn, err := net.DialTimeout("tcp", server_ip+":"+config.OPEN_UDP_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "Open TCP connection to get genericCC port number") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, timeRTTMap, true
	}

	defer conn.Close()

	// create UDP listening port
	srcport := strconv.Itoa((9875 - cycle))
	conn.Write([]byte(alg + "->" + srcport)) // write remy
	//log.WithFields(log.Fields{"port": srcport}).Info("Listening on src port")

	n, err := conn.Read(recvBuf)
	if CheckErrMsg(err, "Trying to receive port number from genericCC") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, timeRTTMap, true
	}
	gccPort := string(recvBuf[:n])
	//log.WithFields(log.Fields{"port": gccPort}).Info("Received port number genericCC will be running on")

	laddr, err := net.ResolveUDPAddr("udp", ":"+srcport) // listen at a known port for later udp messages
	if CheckErrMsg(err, "creating laddr") {
		return flow_throughputs, flow_times, timeRTTMap, true
	}

	receiver, err := net.ListenUDP("udp", laddr)
	if CheckErrMsg(err, "error on creating receiver for listen UDP") {
		return flow_throughputs, flow_times, timeRTTMap, true
	}
	defer receiver.Close()

	// start listening for genericCC
	gccAddr, err := net.ResolveUDPAddr("udp", server_ip+":"+gccPort)
	if CheckErrMsg(err, "resolving addr to generic CC port given") {
		return flow_throughputs, flow_times, timeRTTMap, true
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
					return flow_throughputs, flow_times, timeRTTMap, true
				}
			} else {
				log.Info("Error when punching NAT: ", err)
				return flow_throughputs, flow_times, timeRTTMap, true
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

		bytes_received += uint32(n)
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
	return flow_throughputs, flow_times, timeRTTMap, timed_out
}

func runExperiment(f func(server_ip string, alg string, num_cycles int, cycle int, exp_time time.Duration) (results.BytesTimeMap, results.OnOffMap, results.TimeRTTMap, bool), IP string, alg string, report *results.CCResults, protocol string, port string, num_cycles int, cycle int, exp_time time.Duration) bool {
	throughput := make(results.BytesTimeMap)
	flow_times := make(results.OnOffMap)
	time_map := make(results.TimeRTTMap)
	timed_out := false

	throughput, flow_times, time_map, timed_out = f(IP, alg, num_cycles, cycle, exp_time)

	if !timed_out {
		report.Throughput[alg][cycle] = throughput
		report.FlowTimes[alg][cycle] = flow_times
		report.Delay[alg][cycle] = time_map
	}
	return timed_out

}

func sendReport(report []byte) {
	log.Info("Sending report to server")
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.DB_SERVER_PORT, config.CONNECT_TIMEOUT*time.Second)
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

/*Contact the known DB server for a list of IPs to run the experiment at*/
/*
func getIPS() (results.IPList, []string, int) {
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.IP_SERVER_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckError(err) {
		return make(results.IPList), make([]string, 0), 0
	}
	defer conn.Close()
	ack_buf := []byte("ack")
	recv_buf := make([]byte, config.LARGE_BUF_SIZE)

	conn.Write(ack_buf)

	// write ack, get back list of IPs
	n, err := conn.Read(recv_buf)
	if CheckError(err) {
		return make(results.IPList), make([]string, 0), 0
	}
	ip_list, ip_order, num_cycles := results.DecodeIPList(recv_buf[:n])

	return ip_list, ip_order, num_cycles

}
*/

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

func runExperimentOnMachine(IP string, algs []string, num_cycles int, place int, total_experiments int, record bool, exp_time time.Duration) (string, int) {
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
		err = os.Remove(localResultsStorage)
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
				log.Info("skipping")
				goto saveToFile // continue onto next algorithm and cycle
			} else {
				log.WithFields(log.Fields{"alg": alg, "proto": proto, "server": IP}).Info(fmt.Sprintf("Starting Experiment %d of %d", place+1, total_experiments))
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

			if proto == "tcp" {
				runExperiment(measureTCP, IP, alg, &report, "tcp", config.PING_TCP_SERVER_PORT, 1, cycle, this_exp_time)
			} else if proto == "udp" {
				runExperiment(measureUDP, IP, alg, &report, "udp", config.PING_UDP_SERVER_PORT, 1, cycle, this_exp_time)
			} else {
				log.Error("Unknown protocol!")
			}

			// write report so far into file
		saveToFile:
			b := results.EncodeCCResults(&report)
			err := ioutil.WriteFile(localResultsStorage, b, 0777)
			CheckErrMsg(err, "Writing file into bytes")
			place++
		}
		cycle++
	}

	sendTime := currentTime()
	report.SendTime = sendTime
	sendReport(results.EncodeCCResults(&report))
	// write the file - has a send time
	b := results.EncodeCCResults(&report)
	err := ioutil.WriteFile(localResultsStorage, b, 0777)
	CheckErrMsg(err, "Writing file into bytes")
	return sendTime, place
}

func currentTime() string {
	hour, min, sec := time.Now().Clock()
	return fmt.Sprintf("%d.%d.%d", hour, min, sec)
}

func CheckErrMsg(err error, message string) bool { // check error
	if err != nil {
		log.WithFields(log.Fields{"msg": message}).Error(err)
	}
	return err != nil
}

func getURLFromServer(gg results.GraphInfo) string {
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.DB_GRAPH_PORT, config.CONNECT_TIMEOUT*time.Second)
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

type ServerList []map[string][]string
type YAMLConfig struct {
	Num_cycles int
	Exp_time   string
	Servers    ServerList
}

func ParseYAMLConfig(config_file string) (ServerList, int, time.Duration) {
	config := YAMLConfig{}
	data, err := ioutil.ReadFile(config_file)
	if err != nil {
		log.Warn("Hint: make sure you're only using spaces, not tabs!")
		log.Fatal("Error reading config file: ", err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Warn("Hint: make sure you're only using spaces, not tabs!")
		log.Fatal("Error parsing config file: ", err)
	}
	exp_time, err := time.ParseDuration(config.Exp_time)
	if err != nil {
		log.Fatal("Config contains invalid exp_time, expected format: [0-9]?(s|m|h)")
	}
	return config.Servers, config.Num_cycles, exp_time
}

var use_mm = flag.Bool("mm", false, "If true, connect to a local server from inside a mahimahi shell")
var manual_algs = flag.String("algs", "", "Specify a comma-separated list of algorithms to test, e.g. \"tcp-cubic,tcp-reno\"")
var local_iplist = flag.String("config", "", "Filename to read ips and algorithms from rather than pulling from server")
var should_resume = flag.Bool("resume", false, "Resume from most recent unfinished run")

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if a == b {
			return true
		}
	}
	return false
}

/*Client will do Remy experiment first, then Cubic experiment, then send data back to the server*/
func main() {

	version := "v1.3-c1"
	fmt.Printf("cctest %s\n\n", version)

	flag.Parse()

	log.Info("If this script fails to complete for any reason, re-run with the --resume flag to pick up where you left off.")

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
	algs := []string{"tcp-cubic"}

	if *use_mm {
		mahimahi := os.Getenv("MAHIMAHI_BASE")
		if *manual_algs != "" {
			algs = strings.Split(*manual_algs, ",")
		}
		num_cycles := 1
		runExperimentOnMachine(mahimahi, algs, num_cycles, 0, len(algs)*num_cycles, false, 30*time.Second)
	} else { // contact DB for files
		var servers ServerList
		var num_cycles int
		var exp_time time.Duration
		if *local_iplist != "" {
			if _, err := os.Stat(*local_iplist); os.IsNotExist(err) {
				log.Fatal("Unable to find config file ", *local_iplist)
			}
			servers, num_cycles, exp_time = ParseYAMLConfig(*local_iplist)
		} else {
			log.Fatal("Remote server list not yet supported. Please provide a local config file.")
		}

		sendMap := make(map[string]string) // maps IPs to times the report was sent

		count := 1
		total_experiments := 0
		place := 0
		for _, d := range servers {
			for ip, algs := range d {
				total_experiments += len(algs) * num_cycles
				if stringInSlice(ip, finishedIPs) {
					place += len(algs) * num_cycles
					count++
				}
			}
		}
		for _, d := range servers {
			for ip, algs := range d {
				sendTime := "NONE"
				new_place := 0
				if stringInSlice(ip, finishedIPs) {
					// file should be available to look for send time
					localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, ip)
					sendTime = getSendTimeLocalFile(localResultsStorage)
					sendMap[ip] = sendTime
					continue
				}
				log.WithFields(log.Fields{"ip": ip}).Info(fmt.Sprintf("Contacting Server %d/%d ", count, len(servers)))
				sendTime, new_place = runExperimentOnMachine(ip, algs, num_cycles, place, total_experiments, *should_resume, exp_time)
				place = new_place
				sendMap[ip] = sendTime
				count++

				// the file should also be available, look for sendTime there
				localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, ip)
				sendTime = getSendTimeLocalFile(localResultsStorage)
				sendMap[ip] = sendTime
				fmt.Fprintf(progressWriter, "%s\n", ip)
				progressWriter.Flush()
			}
		}

		// now ask the server for the link to the graph
		log.Info("We will now give links to the graphs summarizing the experiment. They might not load immediately.")
		count = 1
		for IP, time := range sendMap {
			if time == "NONE" {
				continue
			}
			info := results.GraphInfo{ServerIP: IP, SendTime: time}
			url := getURLFromServer(info)
			result := fmt.Sprintf("View result # %d at %s\n", count, url)
			log.Info(result)
			count++
		}
		// delete all the files
		for _, d := range servers {
			for ip, _ := range d {
				localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, ip)
				err := os.Remove(localResultsStorage)
				if err != nil {
					log.Info("Error removing local results storage: ", localResultsStorage)
				}
			}
		}
		// delete progress files
		err := os.Remove(config.LOCAL_PROGRESS_FILE)
		if err != nil {
			log.Info("Error removing local progress file: ", config.LOCAL_PROGRESS_FILE)
		}

	}

	log.Info("All experiments finished! Thanks for helping us with our congestion control research.")
}
