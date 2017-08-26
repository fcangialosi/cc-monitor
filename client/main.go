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
	"sync"
	"time"

	"../config"
	"../results"
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
	//entire_throughput := singleThroughputMeasurement(cur_time, bytes_received)
	// if bytes_received < 20000 {
	// 	log.WithFields(log.Fields{"thr": entire_throughput, "time in program": cur_time, "bytes rec so far": bytes_received}).Info("throughput rec")
	// }
	// m[bytes_received] = entire_throughput
	m[bytes_received] = cur_time
	//log.WithFields(log.Fields{"mbps": singleThroughputMeasurement(time, bytes_received)}).Info()
	/*received := next_measurement
	for received <= bytes_received {
		// add an entry into the map
		m[received] = entire_throughput
		received *= 2
	}*/
	//return received // return the last received throughput
}

/*Sends start tcp message to server and records tcp throughput*/
// NOTE: this function is basically a copy of start_remy, except I didn't want them to use the same function,
// because the remy function requires the echo packet step, and I didn't want to add a condition to check for - if it's tcp or remy (unnecessary time)

func measureTCP2(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int, cycle int) (results.BytesTimeMap, results.OnOffMap, bool) {
	flow_throughputs := results.BytesTimeMap{}
	flow_times := results.OnOffMap{}
	flow_times[config.START] = float32(0)
	flow_times[config.END] = float32(0)

	original_start := time.Now()
	start_ch <- original_start // start pings now
	defer func() { end_ch <- time.Time{} }()

	// loop over each cycle and request TCP server for "1" on and off
	last_received_time := float32(0)
	recvBuf := make([]byte, config.TCP_TRANSFER_SIZE)
	bytes_received := uint32(0)

	// start connection
	conn, err := net.DialTimeout("tcp", server_ip+":"+config.MEASURE_SERVER_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "tcp connection to server") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, true
	}
	conn.Write([]byte(alg))
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
		return flow_throughputs, flow_times, true
	}

	// set first deadline for 30 seconds, then 30 seconds after
	started_flow := false
	dline := time.Now().Add(config.HALF_MINUTE_TIMEOUT * time.Second)
	conn.SetReadDeadline(dline)
	// log.WithFields(log.Fields{"deadline": dline}).Info("set read deadline")
	//conn.SetReadDeadline(time.Now().Add(config.HALF_MINUTE_TIMEOUT * time.Second))

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
			dline := time.Now().Add(30 * time.Second)
			conn.SetReadDeadline(dline)
			// log.WithFields(log.Fields{"deadline": dline}).Info("set read deadline")
			//conn.SetReadDeadline(time.Now().Add(30 * time.Second)) // connection should end 30 seconds from now
		}

		bytes_received += uint32(n)
		last_received_time = elapsed(original_start)
		measureThroughput(start, bytes_received, flow_throughputs)

	}
	log.WithFields(log.Fields{
		"trial":                 cycle + 1,
		"bytes_received":        fmt.Sprintf("%.3f MBytes", float64(bytes_received)/1000000.0),
		"last_received_data_at": time.Duration(flow_throughputs[bytes_received]) * time.Millisecond,
		"time_elapsed":          elapsed(start) / 1000,
		"throughput":            fmt.Sprintf("%.3f Mbit/sec", singleThroughputMeasurement(flow_throughputs[bytes_received], bytes_received)),
	}).Info("Finished Trial")

	flow_times[config.END] = last_received_time
	conn.Close() // close connection before next one

	return flow_throughputs, flow_times, false

}

func measureUDP2(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int, cycle int) (results.BytesTimeMap, results.OnOffMap, bool) {
	timed_out := false
	flow_throughputs := results.BytesTimeMap{}
	flow_times := results.OnOffMap{}
	flow_times[config.START] = float32(0)
	flow_times[config.END] = float32(0)

	// send start to ping channel
	original_start := time.Now()
	start_ch <- original_start
	defer func() { end_ch <- time.Time{} }()

	// for each flow, start a separate connection to the server to spawn genericCC
	bytes_received := uint32(0)
	shouldEcho := (alg[:4] == "remy")
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)

	// create a TCP connection to get the genericCC port
	conn, err := net.DialTimeout("tcp", server_ip+":"+config.OPEN_UDP_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckErrMsg(err, "Open TCP connection to get genericCC port number") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, true
	}
	defer conn.Close()

	// create UDP listening port
	srcport := strconv.Itoa((9875 - cycle))
	conn.Write([]byte(alg + "->" + srcport)) // write remy
	//log.WithFields(log.Fields{"port": srcport}).Info("Listening on src port")

	n, err := conn.Read(recvBuf)
	if CheckErrMsg(err, "Trying to receive port number from genericCC") {
		time.Sleep(2 * time.Second)
		return flow_throughputs, flow_times, true
	}
	gccPort := string(recvBuf[:n])
	//log.WithFields(log.Fields{"port": gccPort}).Info("Received port number genericCC will be running on")

	laddr, err := net.ResolveUDPAddr("udp", ":"+srcport) // listen at a known port for later udp messages
	if CheckErrMsg(err, "creating laddr") {
		return flow_throughputs, flow_times, true
	}

	receiver, err := net.ListenUDP("udp", laddr)
	if CheckErrMsg(err, "error on creating receiver for listen UDP") {
		return flow_throughputs, flow_times, true
	}
	defer receiver.Close()

	// start listening for genericCC
	gccAddr, err := net.ResolveUDPAddr("udp", server_ip+":"+gccPort)
	if CheckErrMsg(err, "resolving addr to generic CC port given") {
		return flow_throughputs, flow_times, true
	}
	start := time.Now()
	flow_start := float32(start.Sub(original_start).Seconds() * 1000)
	last_received_time := flow_start
	flow_times[config.START] = flow_start

	// loop of punching NAT and waiting for a response
	// log.Info("trying to punch NAT and establish connection")

	attempts := 1
	for {
		receiver.WriteToUDP([]byte("open seasame"), gccAddr) // just send, ifnore any errors
		dline := time.Now().Add(config.HALF_MINUTE_TIMEOUT / 6 * time.Second)
		receiver.SetReadDeadline(dline)
		// log.WithFields(log.Fields{"deadline": dline}).Info("set read deadline")
		attempts++
		//receiver.SetReadDeadline(time.Now().Add(config.MINUTE_TIMEOUT / 6 * time.Second))
		n, raddr, err := receiver.ReadFromUDP(recvBuf)
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				if attempts < config.MAX_CONNECT_ATTEMPTS {
					log.Warn("timeout, trying to initiate again...")
					continue
				} else {
					return flow_throughputs, flow_times, true
				}
			} else {
				log.Info("Error when punching NAT: ", err)
				return flow_throughputs, flow_times, true
			}

		}
		// reset start time
		// log.Info("Received first data from UDP program")
		start = time.Now()
		flow_start = float32(start.Sub(original_start).Seconds() * 1000)
		last_received_time = flow_start
		flow_times[config.START] = flow_start
		// send first echo and break
		if shouldEcho {
			//log.Info("echo packet")
			echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, float64(elapsed(start)))
			// TODO can just send back the recvbuf
			receiver.WriteToUDP(echo.Bytes(), raddr)
		}

		break

	}

	// initial timeout -> 30 Seconds
	dline := time.Now().Add(config.HALF_MINUTE_TIMEOUT * time.Second)
	receiver.SetReadDeadline(dline)
	// log.WithFields(log.Fields{"deadline": dline}).Info("set read deadline")
	//receiver.SetReadDeadline(time.Now().Add(config.HALF_MINUTE_TIMEOUT * time.Second))

	for {
		n, raddr, err := receiver.ReadFromUDP(recvBuf)

		if err, ok := err.(net.Error); ok && err.Timeout() {
			// log.Warn("timeout")
			break
		} else if err == io.EOF {
			break
		}

		bytes_received += uint32(n)
		last_received_time = elapsed(original_start)
		measureThroughput(start, bytes_received, flow_throughputs)

		// echo packet with receive timestamp
		if shouldEcho {
			//log.Info("echo packet")
			echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, float64(elapsed(start)))
			// TODO can just send back the recvbuf
			receiver.WriteToUDP(echo.Bytes(), raddr)
		}

	}
	log.WithFields(log.Fields{
		"trial":                 cycle + 1,
		"bytes_received":        fmt.Sprintf("%.3f MBytes", float64(bytes_received)/1000000.0),
		"last_received_data_at": time.Duration(flow_throughputs[bytes_received]) * time.Millisecond,
		"time_elapsed":          elapsed(start) / 1000,
		"throughput":            fmt.Sprintf("%.3f Mbit/sec", singleThroughputMeasurement(flow_throughputs[bytes_received], bytes_received)),
	}).Info("Finished Trial")

	//log.Info("Ending connection and putting in timestamps")
	flow_times[config.END] = last_received_time
	return flow_throughputs, flow_times, timed_out
}

/*Starts a ping chain - stops when the other goroutine sends a message over a channel*/
func sendPings(server_ip string, start_ch chan time.Time, end_ch chan time.Time, protocol string, port string) results.TimeRTTMap {

	rtt_dict := results.TimeRTTMap{}

	start := <-start_ch

	// if protocol is UDP -> use separate connections in goroutine
	if protocol == "udp" {
		var mutex = &sync.Mutex{}
	udpSendloop:
		for {
			select {
			case <-end_ch:
				break udpSendloop
			default:
				go func(m results.TimeRTTMap) {
					conn, err := net.Dial(protocol, server_ip+":"+port)
					if err != nil {
						log.Warn("Non nill error on writing udp pings: ", err)
						return
					}
					defer conn.Close()

					sendTimestamp := elapsed(start)
					conn.Write([]byte(config.ACK))
					recvBuf := make([]byte, config.PING_SIZE_BYTES)
					_, err = conn.Read(recvBuf)
					recvTimestamp := elapsed(start)
					if err != nil {
						log.Warn("Non nil error on udp pings: ", err)
						return
					}

					rtt := recvTimestamp - sendTimestamp
					mutex.Lock()
					m[sendTimestamp] = rtt
					mutex.Unlock()

				}(rtt_dict)

			}
			time.Sleep(time.Millisecond * 500)
		}

		mutex.Lock()
		avg_delay := float32(0)
		for _, rtt := range rtt_dict {
			avg_delay += rtt
		}
		avg_delay /= float32(len(rtt_dict))
		mutex.Unlock()
		log.WithFields(log.Fields{"avg_delay_ms": avg_delay}).Info()
		return rtt_dict
	}

	// protocol is TCP

	conn, err := net.DialTimeout(protocol, server_ip+":"+port, config.CONNECT_TIMEOUT*time.Second)
	if CheckError(err) {
		log.Warn("Error creating connection for pings", err)
		<-end_ch
		return rtt_dict
	}
	defer conn.Close()

	i := 0
sendloop:
	for {
		recvBuf := make([]byte, config.PING_SIZE_BYTES)
		send_timestamp := elapsed(start)
		conn.Write([]byte(strconv.Itoa(i)))
	recvloop:
		for {
			select {
			case <-end_ch:
				break sendloop
			default:
				conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				n, err := conn.Read(recvBuf)
				recv_timestamp := elapsed(start)
				if err != nil || n <= 0 {
					if err, ok := err.(net.Error); ok && !err.Timeout() {
						log.Warn("error reading pings", err)
					}
					continue
				}
				rtt := (recv_timestamp - send_timestamp)
				rtt_dict[send_timestamp] = rtt
				break recvloop
			}
		}
	}

	avg_delay := float32(0)
	for _, rtt := range rtt_dict {
		avg_delay += rtt
	}
	avg_delay /= float32(len(rtt_dict))
	log.WithFields(log.Fields{"avg_delay_ms": avg_delay}).Info()
	return rtt_dict

	/*
				go func(conn net.Conn, end_ch chan time.Time) {
				sendloop:
					for {
						select {
						case <-end_ch:
							break sendloop
						default:
							send_timestamp := elapsed(start)
							payload := strconv.FormatFloat(float64(send_timestamp), 'f', -1, 32) + " "
							conn.Write([]byte(payload))
						}
						time.Sleep(time.Millisecond * config.PING_INTERSEND_MS)
					}
				}(conn, end_ch)

			recvloop:
				for {
					select {
					case <-end_ch:
						break recvloop
					default:
						recvBuf := make([]byte, config.PING_SIZE_BYTES)
						conn.SetReadDeadline(time.Now().Add(1 * time.Second))
						n, err := conn.Read(recvBuf)
						recv_timestamp := elapsed(start)
						if err != nil || n <= 0 {
							if err, ok := err.(net.Error); ok && !err.Timeout() {
								log.Warn("error reading pings", err)
							}
							continue
						}
						log.Info(string(recvBuf[:n]))
						for _, pkt := range strings.Split(string(recvBuf[:n]), " ") {
							if len(pkt) <= 0 {
								continue
							}
							payload, err := strconv.ParseFloat(pkt, 32)
							if CheckErrMsg(err, "unable to parse send timestamp in ping") {
								continue
							}
							send_timestamp := float32(payload)
							rtt := (recv_timestamp - send_timestamp)
							log.Info(rtt)
							rtt_dict[send_timestamp] = rtt
						}
					}
				}
		return rtt_dict
	*/

}

func runExperiment(f func(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int, cycle int) (results.BytesTimeMap, results.OnOffMap, bool), IP string, alg string, report *results.CCResults, protocol string, port string, num_cycles int, cycle int) bool {
	var wg sync.WaitGroup
	start_ping := make(chan time.Time)
	end_ping := make(chan time.Time)
	throughput := make(results.BytesTimeMap)
	flow_times := make(results.OnOffMap)
	ping_results := make(results.TimeRTTMap)
	timed_out := false

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		throughput, flow_times, timed_out = f(IP, alg, start_ping, end_ping, num_cycles, cycle)
	}(&wg)

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		ping_results = sendPings(IP, start_ping, end_ping, protocol, port)
	}(&wg)

	wg.Wait()

	if !timed_out {
		report.Throughput[alg][cycle] = throughput
		report.FlowTimes[alg][cycle] = flow_times
		report.Delay[alg][cycle] = ping_results
	}
	return timed_out

}

func sendReport(report []byte) {
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.DB_SERVER_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckError(err) {
		return
	}
	defer conn.Close()
	conn.Write(report)
}

/*Contact the known DB server for a list of IPs to run the experiment at*/
func getIPS() (results.IPList, int) {
	conn, err := net.DialTimeout("tcp", config.DB_IP+":"+config.IP_SERVER_PORT, config.CONNECT_TIMEOUT*time.Second)
	if CheckError(err) {
		return make(results.IPList), 0
	}
	defer conn.Close()
	ack_buf := []byte("ack")
	recv_buf := make([]byte, config.LARGE_BUF_SIZE)

	conn.Write(ack_buf)

	// write ack, get back list of IPs
	n, err := conn.Read(recv_buf)
	if CheckError(err) {
		return make(results.IPList), 0
	}
	ip_list, num_cycles := results.DecodeIPList(recv_buf[:n])

	return ip_list, num_cycles

}

//function to get the public ip address - found online
func GetOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if CheckError(err) {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().String()
	idx := strings.LastIndex(localAddr, ":")
	return localAddr[0:idx]
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

func runExperimentOnMachine(IP string, algs []string, num_cycles int, place int, total_experiments int, record bool) (string, int) {
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
			// log.Info("Len of saved bytes ", len(savedBytes))
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

	// open a temporary file to write the results in

	for _, full_alg := range algs {
		alg := strings.SplitN(full_alg, "-", 2)[1]
		report.Throughput[alg] = make([]results.BytesTimeMap, num_cycles)
		report.Delay[alg] = make([]results.TimeRTTMap, num_cycles)
		report.FlowTimes[alg] = make([]results.OnOffMap, num_cycles)
	}

	// go through the temporary
	// have loop for cycles here
	cycle := 0
	for cycle < num_cycles {
		// loop through each algorithm
		for _, alg := range algs {
			algLineSplit := strings.Split(alg, "-")
			proto := strings.ToLower(algLineSplit[0])
			alg := strings.ToLower(strings.Join(algLineSplit[1:], "-"))

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

			if proto == "tcp" {
				runExperiment(measureTCP2, IP, alg, &report, "tcp", config.PING_TCP_SERVER_PORT, 1, cycle)
				//runExperiment(measureUDP2, IP, "remy=bigbertha-100x.dna.5", &report, "udp", config.PING_TCP_SERVER_PORT, 1, cycle)
			} else if proto == "udp" {
				runExperiment(measureUDP2, IP, alg, &report, "udp", config.PING_UDP_SERVER_PORT, 1, cycle)
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
	log.Info("Sending report to server")
	sendReport(results.EncodeCCResults(&report))
	// write the file - has a send time
	b := results.EncodeCCResults(&report)
	err := ioutil.WriteFile(localResultsStorage, b, 0777)
	CheckErrMsg(err, "Writing file into bytes")
	// TODO: write version that writes progress into "progress file" as well
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

var use_mm = flag.Bool("mm", false, "If true, connect to a local server from inside a mahimahi shell")
var manual_algs = flag.String("algs", "", "Specify a comma-separated list of algorithms to test, e.g. \"tcp-cubic,tcp-reno\"")
var cycles = flag.Int("cycles", 0, "Specify number of trials for each algorithms")
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

	version := "v1.0-c12"
	fmt.Printf("cctest %s\n\n", version)

	flag.Parse()

	log.Info("This script will contact different servers to transfer data using different congestion control algorithms, and records data about the performance of each algorithm. It may take around 10 minutes. We're trying to guage the performance of an algorithm designed by Remy, a program that automatically generates congestion control algorithms based on input parameters.")
	log.Warn("In case the script doesn't run fully, it will write partial results to /tmp/cc-client_results-IP.log and /tmp/cc-client_progress.log in order to checkpoint progress. Next time the script runs, it will pick up from this progress")
	// look for a local progress file -> just lists IPs the results have been sent to
	// on completing a full run, will delete the file
	finishedIPs := make([]string, 0)
	var progressFile *os.File
	if _, err := os.Stat(config.LOCAL_PROGRESS_FILE); *should_resume && err == nil {
		// read the IPs of this file into the finishedIPs - so we don't run those experiments
		progressFile, err = os.OpenFile(config.LOCAL_PROGRESS_FILE, os.O_RDWR, 0644)
		CheckErrMsg(err, "opening local progress file")
		scanner := bufio.NewScanner(progressFile)
		// log.Info("local progress file exists")
		for scanner.Scan() {
			finishedIPs = append(finishedIPs, scanner.Text())
		}
	} else {
		log.Info("Opening file to record local progress: ", config.LOCAL_PROGRESS_FILE)
		progressFile, err = os.OpenFile(config.LOCAL_PROGRESS_FILE, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644) // also creates file
		CheckErrMsg(err, "Creating file to record local progress")
	}

	defer progressFile.Close()
	progressWriter := bufio.NewWriter(progressFile)

	// Default to just Remy and TCP Cubic
	algs := []string{"udp-remy=bigbertha-100x.dna.5", "tcp-cubic"}

	if *use_mm {
		mahimahi := os.Getenv("MAHIMAHI_BASE")
		if *manual_algs != "" {
			algs = strings.Split(*manual_algs, ",")
		}
		num_cycles := 2
		if *cycles != 0 {
			num_cycles = *cycles
		}
		runExperimentOnMachine(mahimahi, algs, num_cycles, 0, len(algs)*num_cycles, false)
	} else { // contact DB for files
		var ip_map map[string][]string
		var num_cycles int
		if *local_iplist != "" {
			// TODO check if file eists, error if not
			// TODO read file, error if bad format
			ip_map, num_cycles = results.GetIPList(*local_iplist)
		} else {
			ip_map, num_cycles = getIPS()
		}
		if *cycles != 0 {
			num_cycles = *cycles
		}

		sendMap := make(map[string]string) // maps IPs to times the report was sent

		count := 1
		total_experiments := 0
		place := 0
		for ip, val := range ip_map {
			total_experiments += len(val) * num_cycles
			if stringInSlice(ip, finishedIPs) {
				place += len(val) * num_cycles
				count++
			}
		}
		for ip, val := range ip_map {
			sendTime := "NONE"
			new_place := 0
			if stringInSlice(ip, finishedIPs) {
				continue
			}
			log.WithFields(log.Fields{"ip": ip}).Info(fmt.Sprintf("Contacting Server %d/%d ", count, len(ip_map)))
			sendTime, new_place = runExperimentOnMachine(ip, val, num_cycles, place, total_experiments, *should_resume)
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
		for IP := range ip_map {
			localResultsStorage := fmt.Sprintf("%s-%s.log", config.LOCAL_RESULTS_FILE, IP)
			err := os.Remove(localResultsStorage)
			if err != nil {
				log.Info("Error removing local results storage: ", localResultsStorage)
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
