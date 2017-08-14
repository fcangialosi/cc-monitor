package main

import (
	"encoding/binary"
	"fmt"
	"io"
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

func measureTCP2(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int) ([]results.BytesTimeMap, []results.OnOffMap, bool) {
	flow_throughputs := make([]results.BytesTimeMap, num_cycles)
	flow_times := make([]results.OnOffMap, num_cycles)

	k := 0
	for k < num_cycles {
		flow_throughputs[k] = results.BytesTimeMap{}
		flow_times[k] = results.OnOffMap{}
		flow_times[k][config.START] = float32(0)
		flow_times[k][config.END] = float32(0)
		k++
	}

	original_start := time.Now()
	start_ch <- original_start // start pings now

	// loop over each cycle and request TCP server for "1" on and off
	current_flow := 0
	for current_flow < num_cycles {
		last_received_time := float32(0)
		recvBuf := make([]byte, config.TCP_TRANSFER_SIZE)
		bytes_received := uint32(0)

		// start connection
		conn, err := net.Dial("tcp", server_ip+":"+config.MEASURE_SERVER_PORT)
		if CheckErrMsg(err, "tcp connection to server") {
			time.Sleep(2 * time.Second)
			continue
		}
		conn.Write([]byte(alg))
		// now wait for start
		n, err := conn.Read(recvBuf)
		if string(recvBuf[:n]) != config.START_FLOW {
			log.Error("Did not receive start from server")
		}
		// now start the timer
		start := time.Now() // start of flow is when client sends first message to send back data
		flow_times[current_flow][config.START] = float32(start.Sub(original_start).Seconds() * 1000)
		conn.Write([]byte(config.ACK))
		if CheckErrMsg(err, "opening TCP connection") {
			time.Sleep(2 * time.Second)
			continue
		}

		// set first deadline for 30 seconds, then 30 seconds after
		started_flow := false
		conn.SetReadDeadline(time.Now().Add(config.HALF_MINUTE_TIMEOUT * time.Second))
		for {
			//log.Info("Waiting to read")

			n, err := conn.Read(recvBuf)
			//log.Info("read")

			if err == io.EOF || n <= 0 {
				log.Warn("Server closed connection")
				break
			} else if err, ok := err.(net.Error); ok && err.Timeout() {
				break
			} else if err != nil {
				log.Error(err)
			}

			if !started_flow {
				started_flow = true
				conn.SetReadDeadline(time.Now().Add(30 * time.Second)) // connection should end 30 seconds from now
			}

			bytes_received += uint32(n)
			last_received_time = elapsed(original_start)
			measureThroughput(start, bytes_received, flow_throughputs[current_flow])

		}
		log.WithFields(log.Fields{
			"trial":                 current_flow + 1,
			"bytes_received":        fmt.Sprintf("%.3f MBytes", float64(bytes_received)/1000000.0),
			"last_received_data_at": time.Duration(flow_throughputs[current_flow][bytes_received]) * time.Millisecond,
			"time_elapsed":          elapsed(start) / 1000,
			"throughput":            fmt.Sprintf("%.3f Mbit/sec", singleThroughputMeasurement(flow_throughputs[current_flow][bytes_received], bytes_received)),
		}).Info("Finished Trial")

		flow_times[current_flow][config.END] = last_received_time
		conn.Close() // close connection before next one
		current_flow++

		// sleep for some time
		// 8/10/17: hari requested no sleep time
		// time.Sleep(time.Second * 5)
	}

	end_ch <- time.Time{} // can stop sending pings
	return flow_throughputs, flow_times, false

}

func measureUDP2(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int) ([]results.BytesTimeMap, []results.OnOffMap, bool) {
	flow_throughputs := make([]results.BytesTimeMap, num_cycles)
	flow_times := make([]results.OnOffMap, num_cycles)
	timed_out := false
	k := 0
	for k < num_cycles {
		flow_throughputs[k] = results.BytesTimeMap{}
		flow_times[k] = results.OnOffMap{}
		flow_times[k][config.START] = float32(0)
		flow_times[k][config.END] = float32(0)
		k++
	}

	// send start to ping channel
	original_start := time.Now()
	start_ch <- original_start

	// for each flow, start a separate connection to the server to spawn genericCC
	for flow := 0; flow < num_cycles; flow++ {
		bytes_received := uint32(0)
		shouldEcho := (alg[:4] == "remy")
		recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)

		// create a TCP connection to get the genericCC port
		conn, err := net.Dial("tcp", server_ip+":"+config.OPEN_UDP_PORT)
		if CheckErrMsg(err, "Open TCP connection to get genericCC port number") {
			time.Sleep(2 * time.Second)
			continue
		}
		// create UDP listening port
		srcport := strconv.Itoa((9876 - flow))
		conn.Write([]byte(alg + "->" + srcport)) // write remy
		//log.WithFields(log.Fields{"port": srcport}).Info("Listening on src port")

		n, err := conn.Read(recvBuf)
		if CheckErrMsg(err, "Trying to receive port number from genericCC") {
			time.Sleep(2 * time.Second)
			continue
		}
		gccPort := string(recvBuf[:n])
		//log.WithFields(log.Fields{"port": gccPort}).Info("Received port number genericCC will be running on")

		laddr, err := net.ResolveUDPAddr("udp", ":"+srcport) // listen at a known port for later udp messages
		if CheckErrMsg(err, "creating laddr") {
			continue
		}

		receiver, err := net.ListenUDP("udp", laddr)
		if CheckErrMsg(err, "error on creating receiver for listen UDP") {
			continue
		}

		// start listening for genericCC
		gccAddr, err := net.ResolveUDPAddr("udp", server_ip+":"+gccPort)
		if CheckErrMsg(err, "resolving addr to generic CC port given") {
			continue
		}

		// punch hole in NAT for genericCC
		_, err = receiver.WriteToUDP([]byte("open seasame"), gccAddr) // this could error but that's ok
		_, err = receiver.WriteToUDP([]byte("open seasame"), gccAddr) // this could error but that's ok
		_, err = receiver.WriteToUDP([]byte("open seasame"), gccAddr) // this could error but that's ok
		if CheckErrMsg(err, "Punching NAT for genericCC") {
			time.Sleep(2 * time.Second)
			continue
		}
		//log.Info("Open Sesame!: ", gccAddr)

		// write ACK to server to server can start genericCC
		conn.Write([]byte(config.ACK))

		start := time.Now()
		flow_start := float32(start.Sub(original_start).Seconds() * 1000)
		last_received_time := flow_start
		flow_times[flow][config.START] = flow_start
		// initial timeout -> 30 Seconds
		receiver.SetReadDeadline(time.Now().Add(config.MINUTE_TIMEOUT * time.Second))

		for {
			//log.Info("Waiting to read back on the socket")
			n, raddr, err := receiver.ReadFromUDP(recvBuf)
			//log.Info("read on the socket")

			if err, ok := err.(net.Error); ok && err.Timeout() {
				break
			} else if err == io.EOF {
				break
			}

			if ReadHeaderVal(recvBuf, config.SEQNUM_START, config.SEQNUM_END, binary.LittleEndian) == -1 {
				//log.Info("Read start flow packet")
				log.WithFields(log.Fields{"time now": elapsed(start) / 1000}).Info("Starting 30 second timer")
				receiver.SetReadDeadline(time.Now().Add(config.HALF_MINUTE_TIMEOUT * time.Second)) // should be done 30 seconds from now
			}

			bytes_received += uint32(n)
			last_received_time = elapsed(original_start)
			measureThroughput(start, bytes_received, flow_throughputs[flow])

			// echo packet with receive timestamp
			if shouldEcho {
				//log.Info("echo packet")
				echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, elapsed(start))
				// TODO can just send back the recvbuf
				receiver.WriteToUDP(echo.Bytes(), raddr)
			}

		}
		log.WithFields(log.Fields{
			"trial":                 flow + 1,
			"bytes_received":        fmt.Sprintf("%.3f MBytes", float64(bytes_received)/1000000.0),
			"last_received_data_at": time.Duration(flow_throughputs[flow][bytes_received]) * time.Millisecond,
			"time_elapsed":          elapsed(start) / 1000,
			"throughput":            fmt.Sprintf("%.3f Mbit/sec", singleThroughputMeasurement(flow_throughputs[flow][bytes_received], bytes_received)),
		}).Info("Finished Trial")

		// close the connection to the TCP server and listening on UDP port
		conn.Close()
		receiver.Close()
		//log.Info("Ending connection and putting in timestamps")
		flow_times[flow][config.END] = last_received_time
	}
	end_ch <- time.Time{} // can stop sending pings
	return flow_throughputs, flow_times, timed_out
}

/*Starts a ping chain - stops when the other goroutine sends a message over a channel*/
func sendPings(server_ip string, start_ch chan time.Time, end_ch chan time.Time, protocol string, port string) results.TimeRTTMap {
	//log.Info("entering the send ping function")
	rtt_dict := results.TimeRTTMap{}
	pingBuf := make([]byte, config.PING_SIZE_BYTES)
	if protocol == config.UDP {

		// create tcp connection to the server
		// TODO this should probably use tcp/udp depending on which algorithm is running
		// TODO but need to support this on the server as well, for now just udp
		// conn, err := net.Dial("udp", config.SERVER_IP+":"+config.PING_SERVER_PORT)
		// CheckError(err)
		// defer conn.Close()
		var mutex = &sync.Mutex{}

		// wait for measurement to start
		//log.Info("waiting to receive go in ping function")
		start := <-start_ch
		//log.Info("Got start to send pings")
	sendloop:
		for {
			select {
			case <-end_ch:
				//log.Warn("Got signal to end pings")
				break sendloop
			default:
				go func(m results.TimeRTTMap) {

					recvBuf := make([]byte, config.PING_SIZE_BYTES)
					c, err := net.Dial(protocol, server_ip+":"+port)
					if CheckError(err) {
						return
					}
					defer c.Close()
					//log.Info("Waiting to write ping")
					send_timestamp := elapsed(start)
					c.Write(pingBuf)
					//log.Info("Waiting to read from ping buf")
					_, err = c.Read(recvBuf)
					recv_timestamp := elapsed(start)
					if CheckError(err) {
						return
					}
					rtt := (recv_timestamp - send_timestamp)
					mutex.Lock()
					m[send_timestamp] = rtt
					mutex.Unlock()
				}(rtt_dict)
			}
			time.Sleep(time.Millisecond * 500)
		}
	} else { // tcp connection
		conn, err := net.Dial("tcp", server_ip+":"+port)
		if CheckError(err) {
			return rtt_dict
		}
		defer conn.Close()

		start := <-start_ch // wait for start
		//log.WithFields(log.Fields{"original start": start}).Info("TCP ping times")

		i := 0
	sendloop_tcp:
		for {
			select {
			case <-end_ch:
				//log.Debug("Got signal to end pings")
				break sendloop_tcp
			default:
				recvBuf := make([]byte, config.PING_SIZE_BYTES)
				send_timestamp := elapsed(start)
				//log.WithFields(log.Fields{"send time": send_timestamp, "i": i}).Info("TCP-ping send time")
				conn.Write([]byte(strconv.Itoa(i)))
				_, err = conn.Read(recvBuf)
				if CheckErrMsg(err, "read on tcp pings") {
					continue
				}
				recv_timestamp := elapsed(start)
				rtt := (recv_timestamp - send_timestamp)
				//log.WithFields(log.Fields{"recv time": recv_timestamp, "rtt": rtt, "bytes": n, "buf": string(recvBuf[:n])}).Info("TCP ping times")
				rtt_dict[send_timestamp] = rtt
			}
			time.Sleep(time.Millisecond * 500)
			i += 1
		}
	}
	return rtt_dict
}

func runExperiment(f func(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int) ([]results.BytesTimeMap, []results.OnOffMap, bool), IP string, alg string, report *results.CCResults, protocol string, port string, num_cycles int) bool {
	var wg sync.WaitGroup
	start_ping := make(chan time.Time)
	end_ping := make(chan time.Time)
	throughput := make([]results.BytesTimeMap, num_cycles)
	flow_times := make([]results.OnOffMap, num_cycles)
	ping_results := results.TimeRTTMap{}
	timed_out := false

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		throughput, flow_times, timed_out = f(IP, alg, start_ping, end_ping, num_cycles)
	}(&wg)

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		ping_results = sendPings(IP, start_ping, end_ping, protocol, port)
	}(&wg)

	wg.Wait()

	if !timed_out {
		report.Throughput[alg] = throughput
		report.FlowTimes[alg] = flow_times
		report.Delay[alg] = ping_results
	}
	return timed_out

}

func sendReport(report []byte) {
	conn, err := net.Dial("tcp", config.DB_IP+":"+config.DB_SERVER_PORT)
	if CheckError(err) {
		return
	}
	defer conn.Close()
	conn.Write(report)
}

/*Contact the known DB server for a list of IPs to run the experiment at*/
func getIPS() (results.IPList, int) {
	conn, err := net.Dial("tcp", config.DB_IP+":"+config.IP_SERVER_PORT)
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

func runExperimentOnMachine(IP string, algs []string, num_cycles int, place int, total_experiments int) (string, int) {

	// runs the experiment on the given machine, and uploads the results to the DB server
	// addresses and algorithms to test
	client_ip := GetOutboundIP()

	report := results.CCResults{
		ServerIP:   IP,
		ClientIP:   client_ip,
		Throughput: make(map[string]([]results.BytesTimeMap)),
		Delay:      make(map[string]results.TimeRTTMap),
		FlowTimes:  make(map[string][]results.OnOffMap)}

	for _, alg := range algs {
		alg_line_split := strings.Split(alg, "-")
		proto := strings.ToLower(alg_line_split[0])
		alg := strings.ToLower(strings.Join(alg_line_split[1:], "-"))
		log.WithFields(log.Fields{"alg": alg}).Info("Alg is")
		log.WithFields(log.Fields{"alg": alg, "proto": proto, "server": IP}).Info(fmt.Sprintf("Starting Experiment %d of %d", place+1, total_experiments))

		if proto == "tcp" {
			runExperiment(measureTCP2, IP, alg, &report, "tcp", config.PING_TCP_SERVER_PORT, num_cycles)
		} else if proto == "udp" {
			runExperiment(measureUDP2, IP, alg, &report, "udp", config.PING_UDP_SERVER_PORT, num_cycles)
		} else {
			log.Error("Unknown protocol!")
		}

		place += 1

	}

	// print the reports

	//log.Info("sending report")
	// add in the current time and send in the report
	sendTime := currentTime()
	report.SendTime = sendTime
	log.Info("Sending report to server")
	sendReport(results.EncodeCCResults(&report))
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
	conn, err := net.Dial("tcp", config.DB_IP+":"+config.DB_GRAPH_PORT)
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

/*Client will do Remy experiment first, then Cubic experiment, then send data back to the server*/
func main() {
	// bootstrap -- ask one known server for a list of other server IP
	if false {
		mahimahi := os.Getenv("MAHIMAHI_BASE")
		algs := []string{"remy=bigbertha-100x.dna.5", "cubic"}
		runExperimentOnMachine(mahimahi, algs, config.NUM_CYCLES, 0, len(algs))
	} else {
		ip_map, num_cycles := getIPS()
		sendMap := make(map[string]string) // maps IPs to times the report was sent
		log.Info("This script will contact different servers to transfer data using different congestion control algorithms, and records data about the performance of each algorithm. It may take around 10 minutes. We're trying to guage the performance of an algorithm designed by Remy, a program that automatically generates congestion control algorithms based on input parameters.")
		count := 1
		total_experiments := 0
		place := 0
		for _, val := range ip_map {
			total_experiments += len(val)
		}
		for IP, val := range ip_map {
			log.WithFields(log.Fields{"ip": IP}).Info(fmt.Sprintf("Contacting Server %d/%d ", count, len(ip_map)))
			sendTime, new_place := runExperimentOnMachine(IP, val, num_cycles, place, total_experiments)
			place = new_place
			sendMap[IP] = sendTime
			count++
		}

		// now ask the server for the link to the graph
		log.Info("We will now give links to the graphs summarizing the experiment. They might not load immediately.")
		count = 1
		for IP, time := range sendMap {
			info := results.GraphInfo{ServerIP: IP, SendTime: time}
			url := getURLFromServer(info)
			result := fmt.Sprintf("View result # %d at %s\n", count, url)
			log.Info(result)
			count++
		}
	}

	log.Info("All experiments finished! Thanks for helping us with our congestion control research.")
}
