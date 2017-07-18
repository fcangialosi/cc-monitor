package main

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"../config"
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
	log.WithFields(log.Fields{"mbps": singleThroughputMeasurement(time, bytes_received)}).Info()
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

func measureTCP(alg string, ch chan time.Time) map[float64]float64 {
	throughput_dict := map[float64]float64{}
	bytes_received := float64(0)
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)
	next_measurement := float64(1000)

	conn, err := net.Dial("tcp", config.SERVER_IP+":"+config.MEASURE_SERVER_PORT)
	CheckError(err)
	defer conn.Close()

	start := time.Now()
	conn.Write([]byte(alg))
	ch <- start

	for {
		n, err := conn.Read(recvBuf)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				log.Error(err)
			}
		}

		// measure throughput
		bytes_received += float64(n)
		next_measurement = measureThroughput(start, bytes_received, throughput_dict, next_measurement)
	}
	ch <- time.Time{} // can stop sending pings
	return throughput_dict
}

/*Very similar to start tcp, except sends back a packet with the rec. timestamp*/
func measureUDP(alg string, ch chan time.Time) map[float64]float64 {
	throughput_dict := map[float64]float64{} // returns a map of bytes so far to throughput at that time
	bytes_received := float64(0)
	recvBuf := make([]byte, config.TRANSFER_BUF_SIZE)
	shouldEcho := (alg == "remy")
	next_measurement := float64(1000)

	// create connection
	laddr, err := net.ResolveUDPAddr("udp", ":98765")
	receiver, err := net.ListenUDP("udp", laddr)
	CheckError(err)
	defer receiver.Close()

	raddr, err := net.ResolveUDPAddr("udp", config.SERVER_IP+":"+config.MEASURE_SERVER_PORT)
	CheckError(err)
	// send the start message, then wait for data
	start := time.Now()
	receiver.WriteToUDP([]byte(alg), raddr)
	ch <- start

	// loop to read bytes and send back to the server
	for {
		n, raddr, err := receiver.ReadFromUDP(recvBuf)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				log.Error(err)
			}
		}
		// TODO maybe do something better here
		if string(recvBuf[:3]) == "end" {
			break
		}

		// measure throughput
		bytes_received += float64(n)
		next_measurement = measureThroughput(start, bytes_received, throughput_dict, next_measurement)

		// echo packet with receive timestamp
		if shouldEcho {
			echo := SetHeaderVal(recvBuf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, elapsed(start))
			// TODO can just send back the recvbuf
			receiver.WriteToUDP(echo.Bytes(), raddr)
		}
	}
	ch <- time.Time{} // can stop sending pings
	return throughput_dict
}

/*Starts a ping chain - stops when the other goroutine sends a message over a channel*/
func sendPings(ch chan time.Time) map[float64]float64 {
	rtt_dict := map[float64]float64{}
	pingBuf := make([]byte, config.PING_SIZE_BYTES)
	recvBuf := make([]byte, config.PING_SIZE_BYTES)

	// create tcp connection to the server
	// TODO this should probably use tcp/udp depending on which algorithm is running
	// TODO but need to support this on the server as well, for now just udp
	conn, err := net.Dial("udp", config.SERVER_IP+":"+config.PING_SERVER_PORT)
	CheckError(err)
	defer conn.Close()

	// wait for measurement to start
	start := <-ch

sendloop:
	for {
		select {
		case <-ch:
			break sendloop
		default:
			send_timestamp := elapsed(start)
			conn.Write(pingBuf)
			_, err := conn.Read(recvBuf)
			recv_timestamp := elapsed(start)
			CheckError(err)
			rtt_dict[send_timestamp] = (recv_timestamp - send_timestamp)
			time.Sleep(time.Millisecond * 500)
		}
	}
	return rtt_dict
}

func runExperiment(f func(alg string, ch chan time.Time) map[float64]float64, alg string, report *CCResults) {
	var wg sync.WaitGroup
	end_ping := make(chan time.Time)
	throughput := map[float64]float64{}
	ping_results := map[float64]float64{}

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		throughput = f(alg, end_ping)
	}(&wg)

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		ping_results = sendPings(end_ping)
	}(&wg)

	wg.Wait()

	report.Throughput[alg] = throughput
	report.Delay[alg] = ping_results
}

func sendReport(report []byte) {
	conn, err := net.Dial("tcp", config.SERVER_IP+":"+config.DB_SERVER_PORT)
	CheckError(err)
	defer conn.Close()
	conn.Write(report)
}

/*Client will do Remy experiment first, then Cubic experiment, then send data back to the server*/
func main() {
	// TODO bootstrap -- ask one known server for a list of other server IP
	// addresses and algorithms to test
	udp_algorithms := []string{"remy"}
	tcp_algorithms := []string{"cubic"}
	// TODO shuffle order

	report := CCResults{
		Throughput: make(map[string]map[float64]float64),
		Delay:      make(map[string]map[float64]float64),
	}

	for _, alg := range tcp_algorithms {
		log.WithFields(log.Fields{"alg": alg}).Info("starting experiment")
		runExperiment(measureTCP, alg, &report)
	}
	log.Info(report)
	for _, alg := range udp_algorithms {
		log.WithFields(log.Fields{"alg": alg}).Info("starting experiment")
		runExperiment(measureUDP, alg, &report)
	}
	log.Info("all experiments finished")

	log.Info("sending report")
	sendReport(report.encode())
	log.Info("done")
}
