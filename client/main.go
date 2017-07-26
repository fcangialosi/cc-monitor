package main

import (
	"encoding/binary"
	"io"
	"net"
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
func elapsed(start time.Time) float64 {
	return float64(time.Since(start).Seconds() * 1000)
}

/*Used by both Remy and TCP*/
func singleThroughputMeasurement(t float64, bytes_received float64) float64 {
	return (bytes_received * config.BYTES_TO_MBITS) / (t / 1000) // returns in Mbps
}

/*Measure throughput at increments*/
func measureThroughput(start time.Time, bytes_received float64, m map[float64]float64, next_measurement float64) float64 {
	time := elapsed(start)
	// log.WithFields(log.Fields{"mbps": singleThroughputMeasurement(time, bytes_received)}).Info()
	received := next_measurement
	for received <= bytes_received {
		// add an entry into the map
		m[received] = singleThroughputMeasurement(time, received)
		received *= 2
	}
	return received // return the last received throughput
}

/*Sends start tcp message to server and records tcp throughput*/
// NOTE: this function is basically a copy of start_remy, except I didn't want them to use the same function,
// because the remy function requires the echo packet step, and I didn't want to add a condition to check for - if it's tcp or remy (unnecessary time)

func measureTCP(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time) ([]map[float64]float64, []map[string]float64, bool) {
	flow_throughputs := make([]map[float64]float64, config.NUM_CYCLES)
	bytes_received := float64(0)
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)
	next_measurement := float64(1000)
	flow_times := make([]map[string]float64, config.NUM_CYCLES)
	current_flow := -1
	end_flow_times := make([]float64, config.NUM_CYCLES+1)
	k := 0
	for k < config.NUM_CYCLES {
		flow_throughputs[k] = map[float64]float64{}
		flow_times[k] = map[string]float64{}
		flow_times[k][config.START] = float64(0)
		flow_times[k][config.END] = float64(0)
		k++
	}

	conn, err := net.Dial("tcp", server_ip+":"+config.MEASURE_SERVER_PORT)
	CheckError(err)
	defer conn.Close()

	start := time.Now()
	original_start := start
	last_received_time := elapsed(start)
	conn.Write([]byte(alg))
	start_ch <- start

	for {
		n, err := conn.Read(recvBuf)
		if err == io.EOF {
			log.Info("Received EOF from tcp connection end")
			log.Error(err)
			break // break out of function
		}
		if err != nil {
			log.Error(err)
		}
		if n <= 0 || n >= 3 && string(recvBuf[:n]) == config.FIN {
			log.Info("Got signal to end TCP sending")
			end_flow_times[current_flow+1] = last_received_time // last last recieved time
			break
		}
		if string(recvBuf[:config.START_FLOW_LEN]) == config.START_FLOW {
			current_flow++
			flow_times[current_flow][config.START] = elapsed(original_start)
			end_flow_times[current_flow] = last_received_time
			bytes_received = 0
			start = time.Now()
			next_measurement = 1000 // reset to 1KB
		}

		// measure throughput
		bytes_received += float64(n)
		last_received_time = elapsed(original_start)
		next_measurement = measureThroughput(start, bytes_received, flow_throughputs[current_flow], next_measurement)
	}
	log.Info("Trying to send pings to end tcp measuring")
	end_ch <- time.Time{} // can stop sending pings
	i := 0
	for i < config.NUM_CYCLES {
		flow_times[i][config.END] = end_flow_times[i+1]
		i++
	}
	return flow_throughputs, flow_times, false // last argument is time out argument used for UDP
}

/*Very similar to start tcp, except sends back a packet with the rec. timestamp*/
func measureUDP(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time) ([]map[float64]float64, []map[string]float64, bool) {
	flow_throughputs := make([]map[float64]float64, config.NUM_CYCLES)
	bytes_received := float64(0)
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)
	shouldEcho := (alg == "remy")
	next_measurement := float64(config.INITIAL_X_VAL)
	flow_times := make([]map[string]float64, config.NUM_CYCLES)
	end_flow_times := make([]float64, config.NUM_CYCLES+1)
	current_flow := 0
	started_flow := false
	timed_out := false
	k := 0
	for k < config.NUM_CYCLES {
		flow_throughputs[k] = map[float64]float64{}
		flow_times[k] = map[string]float64{}
		flow_times[k][config.START] = float64(0)
		flow_times[k][config.END] = float64(0)
		k++
	}

	// create connection
	log.Info("Creating listening port")
	laddr, err := net.ResolveUDPAddr("udp", ":9876")
	CheckErrMsg(err, "creating laddr")
	log.Info("create laddr, adding receiver")
	receiver, err := net.ListenUDP("udp", laddr)
	CheckErrMsg(err, "error on creating receiver for listen UDP")
	defer receiver.Close()

	raddr, err := net.ResolveUDPAddr("udp", server_ip+":"+config.MEASURE_SERVER_PORT)
	CheckErrMsg(err, "Error on connecting to UDP on server")
	// SYN : this is the algorithm we want the server to test on us
	receiver.WriteToUDP([]byte(alg), raddr)

	// SYN-ACK : this is the port generiCC will be run on
	n, _, err := receiver.ReadFromUDP(recvBuf)
	gccPort := string(recvBuf[:n])
	gccAddr, err := net.ResolveUDPAddr("udp", server_ip+":"+gccPort)
	CheckErrMsg(err, "resolving addr to generic CC port given")

	// punch hole in NAT for genericCC
	receiver.WriteToUDP([]byte("open seasame"), gccAddr)
	// ACK : also tell server its now allowed to start genericCC
	// receiver.WriteToUDP([]byte(config.ACK), raddr)

	// loop to read bytes and send back to the server
	start := time.Now()
	original_start := start
	last_received_time := elapsed(start)
	start_ping := true
	for {
		// log.Warn("Reading from recv buf in measure UDP")
		// set the read deadline to be well above the off time
		receiver.SetReadDeadline(time.Now().Add(config.CLIENT_TIMEOUT * time.Second))
		log.Info("About to read from recvbuf")
		n, raddr, err := receiver.ReadFromUDP(recvBuf)
		log.Info("Got shit from recv buf")
		// don't start the timer until we've receied the first byte
		// TODO maybe add one RTT here
		if start_ping {
			log.Info("start ping is true")
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
			log.Info("Received FIN")
			end_flow_times[current_flow+1] = last_received_time // last end flow time
			break
		}
		if string(recvBuf[:config.START_FLOW_LEN]) == config.START_FLOW {
			log.Info("Received start flow, incrementing current flow")
			if started_flow {
				current_flow++
			} else {
				started_flow = true
			}
			log.WithFields(log.Fields{"current flow": current_flow}).Info("boohoo")

			flow_times[current_flow][config.START] = elapsed(original_start)
			end_flow_times[current_flow] = last_received_time
			log.WithFields(log.Fields{"last received": last_received_time, "current": elapsed(original_start)}).Info("Received START FLOW GOTTA START A NEW ONE")
			bytes_received = 0
			next_measurement = 1000 // reset to 1KB
			start = time.Now()
		}

		// measure throughput
		bytes_received += float64(n)
		last_received_time = elapsed(original_start)
		next_measurement = measureThroughput(start, bytes_received, flow_throughputs[current_flow], next_measurement)

		// echo packet with receive timestamp
		if shouldEcho {
			log.Info("-----")
			echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, elapsed(start))
			// TODO can just send back the recvbuf
			receiver.WriteToUDP(echo.Bytes(), raddr)
		}
	}
	log.Info("Trying to end signal to stop sending pings channel")
	end_ch <- time.Time{} // can stop sending pings
	log.Info("Returning from the UDP thing")
	i := 0
	for i < config.NUM_CYCLES {
		flow_times[i][config.END] = end_flow_times[i+1]
		i++
	}
	return flow_throughputs, flow_times, timed_out
}

/*Starts a ping chain - stops when the other goroutine sends a message over a channel*/
func sendPings(server_ip string, start_ch chan time.Time, end_ch chan time.Time, protocol string, port string) map[float64]float64 {
	log.Info("entering the send ping function")
	rtt_dict := map[float64]float64{}
	pingBuf := make([]byte, config.PING_SIZE_BYTES)
	recvBuf := make([]byte, config.PING_SIZE_BYTES)

	// create tcp connection to the server
	// TODO this should probably use tcp/udp depending on which algorithm is running
	// TODO but need to support this on the server as well, for now just udp
	// conn, err := net.Dial("udp", config.SERVER_IP+":"+config.PING_SERVER_PORT)
	// CheckError(err)
	// defer conn.Close()
	var mutex = &sync.Mutex{}

	// wait for measurement to start
	log.Info("waiting to receive go in ping function")
	start := <-start_ch
	log.Info("Got start to send pings")
sendloop:
	for {
		select {
		case <-end_ch:
			log.Debug("Got signal to end pings")
			break sendloop
		default:
			go func(m map[float64]float64) {
				mutex.Lock()
				c, err := net.Dial(protocol, server_ip+":"+port)
				defer c.Close()
				CheckError(err)
				// log.Info("Waiting to write ping")
				c.Write(pingBuf)
				send_timestamp := elapsed(start)
				// log.Info("Waiting to read from ping buf")
				_, err = c.Read(recvBuf)
				recv_timestamp := elapsed(start)
				CheckError(err)
				rtt := (recv_timestamp - send_timestamp)
				// mutex.Lock()
				m[send_timestamp] = rtt
				mutex.Unlock()
				//log.WithFields(log.Fields{"protocol": protocol, "rtt": rtt, "sent": send_timestamp}).Warn("Ping Info")
			}(rtt_dict)
		}
		time.Sleep(time.Millisecond * 3000)
	}
	return rtt_dict
}

func runExperiment(f func(server_ip string, alg string, start_ch chan time.Time, end_ch chan time.Time) ([]map[float64]float64, []map[string]float64, bool), IP string, alg string, report *results.CCResults, protocol string, port string) bool {
	var wg sync.WaitGroup
	start_ping := make(chan time.Time)
	end_ping := make(chan time.Time)
	throughput := make([]map[float64]float64, config.NUM_CYCLES)
	flow_times := make([]map[string]float64, config.NUM_CYCLES)
	ping_results := map[float64]float64{}
	timed_out := false

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		throughput, flow_times, timed_out = f(IP, alg, start_ping, end_ping)
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
	log.WithFields(log.Fields{"size": len(report)}).Info("Sending size bytes in chunks")
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
func getIPS() []string {
	conn, err := net.Dial("tcp", config.DB_IP+":"+config.IP_SERVER_PORT)
	defer conn.Close()
	CheckError(err)
	ack_buf := []byte("ack")
	recv_buf := make([]byte, config.LARGE_BUF_SIZE)

	conn.Write(ack_buf)

	// write ack, get back list of IPs
	n, err := conn.Read(recv_buf)
	CheckError(err)
	ip_list := results.DecodeIPList(recv_buf[:n])

	for _, val := range ip_list {
		log.WithFields(log.Fields{"IP": val}).Info("IP")
	}
	return ip_list

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

func runExperimentOnMachine(IP string) {
	// runs the experiment on the given machine, and uploads the results to the DB server
	// addresses and algorithms to test
	udp_algorithms := []string{"remy"}
	tcp_algorithms := []string{"cubic", "bbr"}
	client_ip := GetOutboundIP()

	report := results.CCResults{
		ServerIP:   IP,
		ClientIP:   client_ip,
		Throughput: make(map[string]([]map[float64]float64)),
		Delay:      make(map[string]map[float64]float64),
		FlowTimes:  make(map[string][]map[string]float64)}

	for _, alg := range udp_algorithms {
		log.WithFields(log.Fields{"alg": alg}).Info("starting experiment")
		timed_out := runExperiment(measureUDP, IP, alg, &report, "udp", config.PING_UDP_SERVER_PORT)
		if !timed_out {
			for ind, val := range report.Throughput[alg] {
				log.WithFields(log.Fields{"flow number": ind}).Info("Flow number")
				log.WithFields(log.Fields{"throughput dict": val}).Info("Dict")
			}
			for ind, val := range report.FlowTimes[alg] {
				log.WithFields(log.Fields{"flow number": ind, "flow start": val[config.START], "flow end": val[config.END]}).Info("Flow times")
			}
		} else {
			log.WithFields(log.Fields{"IP": IP}).Warn("UDP sending timed out")
		}

	}
	log.Debug("Finished UDP algorithms")
	for _, alg := range tcp_algorithms {
		log.WithFields(log.Fields{"alg": alg}).Info("starting experiment")
		runExperiment(measureTCP, IP, alg, &report, "tcp", config.PING_TCP_SERVER_PORT)
		for ind, val := range report.Throughput[alg] {
			log.WithFields(log.Fields{"flow number": ind}).Info("Flow number")
			log.WithFields(log.Fields{"throughput dict": val}).Info("Dict")
		}
		for ind, val := range report.FlowTimes[alg] {
			log.WithFields(log.Fields{"flow number": ind, "flow start": val[config.START], "flow end": val[config.END]}).Info("Flow times")
		}
	}

	// print out the TCP results and the UDP results

	log.Debug("Finished TCP algorithms")
	log.Info("all experiments finished")
	// print the reports

	log.Info("sending report")
	sendReport(results.EncodeCCResults(&report))
	log.Info("done")

}

func CheckErrMsg(err error, message string) {
	if err != nil {
		log.WithFields(log.Fields{"msg": message}).Fatal(err)
	}
}

/*Client will do Remy experiment first, then Cubic experiment, then send data back to the server*/
func main() {
	// bootstrap -- ask one known server for a list of other server IP
	ip_list := getIPS()
	log.Info(ip_list)

	for _, IP := range ip_list {
		runExperimentOnMachine(IP)
	}

}
