package main

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	//"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"../config"
	"../results"
	log "github.com/sirupsen/logrus"
)

/*Simple function to print errors or ignore them*/
func CheckError(err error) {
	if err != nil {
		log.Fatal(err)
	}
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
		CheckErrMsg(err, "tcp connection to server")
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
		CheckErrMsg(err, "opening TCP connection")

		for {
			//log.Info("Waiting to read")
			conn.SetReadDeadline(time.Now().Add(config.TCP_TIMEOUT * time.Second))
			n, err := conn.Read(recvBuf)
			//log.Info("read")

			if err == io.EOF || n <= 0 {
				//log.Warn("server closed connection")
				break
			} else if err, ok := err.(net.Error); ok && err.Timeout() {
				//log.Warn("timed out on read")
				break
			} else if err != nil {
				log.Error(err)
			}

			bytes_received += uint32(n)
			last_received_time = elapsed(original_start)
			measureThroughput(start, bytes_received, flow_throughputs[current_flow])

		}
		flow_times[current_flow][config.END] = last_received_time
		conn.Close() // close connection before next one
		current_flow++
		// sleep for some time
		time.Sleep(time.Second * 5)
	}

	end_ch <- time.Time{} // can stop sending pings
	return flow_throughputs, flow_times, false

}

func measureTCP(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int) ([]results.BytesTimeMap, []results.OnOffMap, bool) {
	flow_throughputs := make([]results.BytesTimeMap, num_cycles)
	bytes_received := uint32(0)
	recvBuf := make([]byte, config.TCP_TRANSFER_SIZE)
	//next_measurement := float64(1000)
	flow_times := make([]results.OnOffMap, num_cycles)
	current_flow := -1
	end_flow_times := make([]float32, num_cycles+1)
	k := 0
	for k < num_cycles {
		flow_throughputs[k] = results.BytesTimeMap{}
		flow_times[k] = results.OnOffMap{}
		flow_times[k][config.START] = float32(0)
		flow_times[k][config.END] = float32(0)
		k++
	}

	start := time.Now()
	conn, err := net.Dial("tcp", server_ip+":"+config.MEASURE_SERVER_PORT)
	CheckError(err)
	defer conn.Close()

	original_start := start
	last_received_time := elapsed(start)
	conn.Write([]byte(alg))
	start_ch <- start

	for {
		n, err := conn.Read(recvBuf)
		if err == io.EOF {
			//log.Info("Received EOF from tcp connection end")
			//log.Error(err)
			break // break out of function
		}
		if err != nil {
			log.Error(err)
		}
		if n <= 0 || n >= 3 && string(recvBuf[:n]) == config.FIN {
			//og.Info("Got signal to end TCP sending")
			end_flow_times[current_flow+1] = last_received_time // last last recieved time
			break
		}
		if string(recvBuf[:config.START_FLOW_LEN]) == config.START_FLOW || string(recvBuf[(n-config.START_FLOW_LEN):n]) == config.START_FLOW {
			current_flow++
			flow_times[current_flow][config.START] = elapsed(original_start)
			end_flow_times[current_flow] = last_received_time
			bytes_received = 0
			start = time.Now()
			//next_measurement = 1000 // reset to 1KB
			conn.Write([]byte(config.ACK))
			// do not count this start flow message as the first measurement
			n -= config.START_FLOW_LEN
		}
		// measure throughput
		bytes_received += uint32(n)
		last_received_time = elapsed(original_start)
		measureThroughput(start, bytes_received, flow_throughputs[current_flow])

	}
	//log.Info("Trying to send pings to end tcp measuring")
	end_ch <- time.Time{} // can stop sending pings
	i := 0
	for i < num_cycles {
		flow_times[i][config.END] = end_flow_times[i+1]
		i++
	}
	return flow_throughputs, flow_times, false // last argument is time out argument used for UDP
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
		CheckErrMsg(err, "Open TCP connection to get genericCC port number")
		conn.Write([]byte(alg)) // write remy

		n, err := conn.Read(recvBuf)
		CheckErrMsg(err, "Trying to receive port number from genericCC")
		gccPort := string(recvBuf[:n])
		//log.WithFields(log.Fields{"port": gccPort}).Info("Received port number genericCC will be running on")

		// create UDP listening port
		laddr, err := net.ResolveUDPAddr("udp", ":"+config.CLIENT_UDP_PORT) // listen at a known port for later udp messages
		CheckErrMsg(err, "creating laddr")
		receiver, err := net.ListenUDP("udp", laddr)
		CheckErrMsg(err, "error on creating receiver for listen UDP")

		// start listening for genericCC
		gccAddr, err := net.ResolveUDPAddr("udp", server_ip+":"+gccPort)
		CheckErrMsg(err, "resolving addr to generic CC port given")

		// punch hole in NAT for genericCC
		_, err = receiver.WriteToUDP([]byte("open seasame"), gccAddr) // this could error but that's ok
		CheckErrMsg(err, "Punching NAT for genericCC")
		//log.Info("Open Sesame!")

		// write ACK to server to server can start genericCC
		conn.Write([]byte(config.ACK))

		start := time.Now()
		flow_start := float32(start.Sub(original_start).Seconds() * 1000)
		last_received_time := flow_start
		flow_times[flow][config.START] = flow_start

		for {
			receiver.SetReadDeadline(time.Now().Add(config.CLIENT_TIMEOUT * time.Second)) // long timeout
			n, raddr, err := receiver.ReadFromUDP(recvBuf)

			if err, ok := err.(net.Error); ok && err.Timeout() {
				break
			} else if err == io.EOF {
				break
			}

			// reset the start time to be reasonable upon seeing the -1 packet
			if ReadHeaderVal(recvBuf, config.SEQNUM_START, config.SEQNUM_END, binary.LittleEndian) == -1 {
				start = time.Now()
				flow_start = float32(start.Sub(original_start).Seconds()) * 1000
				last_received_time = flow_start
				flow_times[flow][config.START] = flow_start
				// need to echo the packet
				receiver.WriteToUDP(recvBuf[:n], raddr)
				continue

			}

			bytes_received += uint32(n)
			last_received_time = elapsed(original_start)
			measureThroughput(start, bytes_received, flow_throughputs[flow])

			// echo packet with receive timestamp
			if shouldEcho {
				//log.Info("-----")
				echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, elapsed(start))
				// TODO can just send back the recvbuf
				receiver.WriteToUDP(echo.Bytes(), raddr)
			}

		}

		// close the connection to the TCP server and listening on UDP port
		//log.Info("Ending connection and putting in timestamps")
		flow_times[flow][config.END] = last_received_time
		conn.Close()
		receiver.Close()
	}
	end_ch <- time.Time{} // can stop sending pings
	return flow_throughputs, flow_times, timed_out
}

/*Very similar to start tcp, except sends back a packet with the rec. timestamp*/
func measureUDP(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time, num_cycles int) ([]results.BytesTimeMap, []results.OnOffMap, bool) {
	flow_throughputs := make([]results.BytesTimeMap, num_cycles)
	bytes_received := uint32(0)
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)
	shouldEcho := (alg[:4] == "remy")
	//next_measurement := float64(config.INITIAL_X_VAL)
	flow_times := make([]results.OnOffMap, num_cycles)
	end_flow_times := make([]float32, num_cycles+1)
	current_flow := 0
	started_flow := false
	timed_out := false
	k := 0
	for k < num_cycles {
		flow_throughputs[k] = results.BytesTimeMap{}
		flow_times[k] = results.OnOffMap{}
		flow_times[k][config.START] = float32(0)
		flow_times[k][config.END] = float32(0)
		k++
	}

	// create TCP connection to get the gcc port
	conn, err := net.Dial("tcp", server_ip+":"+config.OPEN_UDP_PORT)
	CheckErrMsg(err, "Open TCP connection to get genericCC port number")
	defer conn.Close()
	conn.Write([]byte(alg)) // write remy

	n, err := conn.Read(recvBuf)
	CheckErrMsg(err, "Trying to receive port number from genericCC")
	gccPort := string(recvBuf[:n])
	//log.WithFields(log.Fields{"port": gccPort}).Info("Received port number genericCC will be running on")

	// create UDP listening port
	laddr, err := net.ResolveUDPAddr("udp", ":"+config.CLIENT_UDP_PORT) // listen at a known port for later udp messages
	CheckErrMsg(err, "creating laddr")
	receiver, err := net.ListenUDP("udp", laddr)
	CheckErrMsg(err, "error on creating receiver for listen UDP")
	defer receiver.Close()

	// start listening on genericCC
	gccAddr, err := net.ResolveUDPAddr("udp", server_ip+":"+gccPort)
	CheckErrMsg(err, "resolving addr to generic CC port given")

	// punch hole in NAT for genericCC
	_, err = receiver.WriteToUDP([]byte("open seasame"), gccAddr) // this could error but that's ok
	if err != nil {
		log.Panic(err)
	}
	//log.Info("Wrote open sesame")

	// write ACK to the server so server can start genericCC
	conn.Write([]byte(config.ACK))

	// loop to read bytes and send back to the server - with genericCC
	start := time.Now()
	original_start := start
	last_received_time := elapsed(start)
	start_ping := true
	for {
		// set the read deadline to be well above the off time
		receiver.SetReadDeadline(time.Now().Add(config.CLIENT_TIMEOUT * time.Second))

		n, raddr, err := receiver.ReadFromUDP(recvBuf)
		//log.Info("read from udp")
		// TODO maybe add one RTT here
		if start_ping {
			start = time.Now()
			start_ch <- start
			start_ping = false
		}

		if err != nil {
			if err == io.EOF {
				break
			} else if err, ok := err.(net.Error); ok && err.Timeout() {
				// timeout error -> break and return an error
				timed_out = true
				break
			} else {
				log.Error(err)
			}
		}
		if string(recvBuf[:config.FIN_LEN]) == config.FIN {
			//log.Info("Received FIN")
			end_flow_times[current_flow+1] = last_received_time // last end flow time
			break
		}
		if ReadHeaderVal(recvBuf, config.SEQNUM_START, config.SEQNUM_END, binary.LittleEndian) == -1 {
			//if string(recvBuf[:config.START_FLOW_LEN]) == config.START_FLOW {
			//log.Info("Received start flow, incrementing current flow")
			if started_flow {
				current_flow++
			} else {
				started_flow = true
			}
			//log.WithFields(log.Fields{"current flow": current_flow}).Info("boohoo")

			flow_times[current_flow][config.START] = elapsed(original_start)
			end_flow_times[current_flow] = last_received_time
			//log.WithFields(log.Fields{"last received": last_received_time, "current": elapsed(original_start)}).Info("Received START FLOW GOTTA START A NEW ONE")
			bytes_received = 0
			//next_measurement = 1000 // reset to 1KB
			start = time.Now()
			// echo packet
			receiver.WriteToUDP(recvBuf[:n], raddr)
			continue
		}

		// measure throughput
		bytes_received += uint32(n)
		last_received_time = elapsed(original_start)
		measureThroughput(start, bytes_received, flow_throughputs[current_flow])

		// echo packet with receive timestamp
		if shouldEcho {
			//log.Info("-----")
			echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, elapsed(start))
			// TODO can just send back the recvbuf
			receiver.WriteToUDP(echo.Bytes(), raddr)
		}
	}
	//log.Info("Trying to end signal to stop sending pings channel")
	end_ch <- time.Time{} // can stop sending pings
	//log.Info("Returning from the UDP thing")
	i := 0
	for i < num_cycles {
		flow_times[i][config.END] = end_flow_times[i+1]
		i++
	}
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
					defer c.Close()
					CheckError(err)
					//log.Info("Waiting to write ping")
					send_timestamp := elapsed(start)
					c.Write(pingBuf)
					//log.Info("Waiting to read from ping buf")
					_, err = c.Read(recvBuf)
					recv_timestamp := elapsed(start)
					CheckError(err)
					rtt := (recv_timestamp - send_timestamp)
					mutex.Lock()
					m[send_timestamp] = rtt
					mutex.Unlock()
					//log.WithFields(log.Fields{"protocol": protocol, "rtt": rtt, "sent": send_timestamp}).Warn("Ping Info")
				}(rtt_dict)
			}
			time.Sleep(time.Millisecond * 500)
		}
	} else { // tcp connection
		conn, err := net.Dial("tcp", server_ip+":"+port)
		CheckError(err)
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
				CheckErrMsg(err, "read on tcp pings")
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
	// send all bytes in 2048 byte  chunks
	// LARGE_BUF_SIZE := 15000
	// ack_buf := make([]byte, len(config.ACK))
	// end_buf := []byte(config.FIN)
	conn, err := net.Dial("tcp", config.DB_IP+":"+config.DB_SERVER_PORT)
	CheckError(err)
	defer conn.Close()
	conn.Write(report)
	//log.WithFields(log.Fields{"size": len(report)}).Info("Sending size bytes in chunks")
	conn.Write(report)
	// bytes_written := 0
	// for bytes_written < len(report) {
	// 	if bytes_written+LARGE_BUF_SIZE >= len(report) {
	// 		log.WithFields(log.Fields{"x": len(report) - bytes_written}).Info("Writing x more bytes")
	// 		conn.Write(report[bytes_written:])
	// 		conn.Read(ack_buf)
	// 		if string(ack_buf) != config.ACK {
	// 			log.Panic("Server did not ack back in report transfer")
	// 		}
	// 		log.Info("About to write the end into the connection")
	// 		conn.Write(end_buf)
	// 		break
	// 	} else {
	// 		conn.Write(report[bytes_written : bytes_written+LARGE_BUF_SIZE])
	// 		conn.Read(ack_buf)
	// 		if string(ack_buf) != config.ACK {
	// 			log.Panic("Server did not ack back in report transfer")
	// 		}
	// 		log.WithFields(log.Fields{"x": config.TRANSFER_BUF_SIZE}).Info("Writing x more bytes")
	// 		bytes_written += config.TRANSFER_BUF_SIZE
	// 	}
	// }
}

/*Contact the known DB server for a list of IPs to run the experiment at*/
func getIPS() (results.IPList, int) {
	conn, err := net.Dial("tcp", config.DB_IP+":"+config.IP_SERVER_PORT)
	defer conn.Close()
	CheckError(err)
	ack_buf := []byte("ack")
	recv_buf := make([]byte, config.LARGE_BUF_SIZE)

	conn.Write(ack_buf)

	// write ack, get back list of IPs
	n, err := conn.Read(recv_buf)
	CheckError(err)
	ip_list, num_cycles := results.DecodeIPList(recv_buf[:n])

	// for key, val := range ip_list {
	// 	log.WithFields(log.Fields{"IP": key, "alg map": val, "num_cycles": num_cycles}).Info("IP")
	// }
	return ip_list, num_cycles

}

//function to get the public ip address - found online
func GetOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	defer conn.Close()
	CheckError(err)
	localAddr := conn.LocalAddr().String()
	idx := strings.LastIndex(localAddr, ":")
	return localAddr[0:idx]
}

func runExperimentOnMachine(IP string, alg_map map[string][]string, num_cycles int) {

	// runs the experiment on the given machine, and uploads the results to the DB server
	// addresses and algorithms to test
	udp_algorithms := alg_map["UDP"]
	tcp_algorithms := alg_map["TCP"]
	// tcp_algorithms = []string{}
	// udp_algorithms = []string{}
	client_ip := GetOutboundIP()

	report := results.CCResults{
		ServerIP:   IP,
		ClientIP:   client_ip,
		Throughput: make(map[string]([]results.BytesTimeMap)),
		Delay:      make(map[string]results.TimeRTTMap),
		FlowTimes:  make(map[string][]results.OnOffMap)}

	for _, alg := range tcp_algorithms {
		//log.WithFields(log.Fields{"alg": alg}).Info("starting experiment")
		runExperiment(measureTCP2, IP, alg, &report, "tcp", config.PING_TCP_SERVER_PORT, num_cycles)
		// for ind, val := range report.Throughput[alg] {
		// 	log.WithFields(log.Fields{"flow number": ind}).Info("Flow number")
		// 	log.WithFields(log.Fields{"throughput dict": val}).Info("Dict")
		// }
		// for ind, val := range report.FlowTimes[alg] {
		// 	log.WithFields(log.Fields{"flow number": ind, "flow start": val[config.START], "flow end": val[config.END]}).Info("Flow times")
		// }
		// for key, val := range report.Delay[alg] {
		// 	log.WithFields(log.Fields{"time sent": key, "rtt": val, "alg": alg}).Info("Ping Times")
		// }
	}
	//log.Debug("Finished TCP algorithms")
	for _, alg := range udp_algorithms {
		//log.WithFields(log.Fields{"alg": alg}).Info("starting experiment")
		runExperiment(measureUDP2, IP, alg, &report, "udp", config.PING_UDP_SERVER_PORT, num_cycles)
		// for ind, val := range report.FlowTimes[alg] {
		// 	log.WithFields(log.Fields{"flow number": ind, "flow start": val[config.START], "flow end": val[config.END]}).Info("Flow times")
		// }
		// for ind, val := range report.Throughput[alg] {
		// 	log.WithFields(log.Fields{"flow number": ind, "length of throughput dict": len(val)}).Info("throughput info")
		// }
		// if !timed_out {
		// 	for ind, val := range report.Throughput[alg] {
		// 		log.WithFields(log.Fields{"flow number": ind}).Info("Flow number")
		// 		log.WithFields(log.Fields{"throughput dict": val}).Info("Dict")
		// 	}
		// 	for ind, val := range report.FlowTimes[alg] {
		// 		log.WithFields(log.Fields{"flow number": ind, "flow start": val[config.START], "flow end": val[config.END]}).Info("Flow times")
		// 	}
		// 	for key, val := range report.Delay[alg] {
		// 		log.WithFields(log.Fields{"time sent": key, "rtt": val, "alg": alg}).Info("Ping Times")
		// 	}
		// } else {
		// 	log.WithFields(log.Fields{"IP": IP}).Warn("UDP sending timed out")
		// }

	}
	//log.Debug("Finished UDP algorithms")

	// print out the TCP results and the UDP results

	// print the reports

	//log.Info("sending report")
	sendReport(results.EncodeCCResults(&report))
}

func CheckErrMsg(err error, message string) {
	if err != nil {
		log.WithFields(log.Fields{"msg": message}).Fatal(err)
	}
}

/*Client will do Remy experiment first, then Cubic experiment, then send data back to the server*/
func main() {
	// bootstrap -- ask one known server for a list of other server IP
	if false {
		mahimahi := os.Getenv("MAHIMAHI_BASE")
		m := make(map[string][]string)
		m["UDP"] = []string{"remy"}
		m["TCP"] = []string{}
		runExperimentOnMachine(mahimahi, m, config.NUM_CYCLES)
	} else {
		ip_map, num_cycles := getIPS()
		log.Info("This script will contact different servers to transfer data using different congestion control algorithms, and records data about the performance of each algorithm. It may take around 15-30 minutes.")
		for IP, val := range ip_map {
			runExperimentOnMachine(IP, val, num_cycles)
		}
	}

	log.Info("All experiments finished! Thanks for helping us with our congestion control research.")
}
