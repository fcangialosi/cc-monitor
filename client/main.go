package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
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
func throughput_so_far(start time.Time, bytes_received float64) float64 {
	return (bytes_received / config.BYTES_TO_MBITS) / elapsed(start) // returns in Mbps
}

/*Sends start tcp message to server and records tcp throughput*/
// NOTE: this function is basically a copy of start_remy, except I didn't want them to use the same function,
// because the remy function requires the echo packet step, and I didn't want to add a condition to check for - if it's tcp or remy (unnecessary time)

func start_tcp(wg sync.WaitGroup, start time.Time, ch chan<- bool) map[float64]float64 {
	defer wg.Done()
	throughput_dict := map[float64]float64{}
	bytes_received := float64(0)
	buf := make([]byte, 2048)
	conn, err := net.Dial("tcp", config.SERVER_IP+":"+config.MEASURE_SERVER_PORT)
	defer conn.Close()
	CheckError(err)
	conn.Write([]byte("tcp"))

	for {
		n, err := conn.Read(buf)
		CheckError(err)

		// current ending condition to close the socket
		if string(buf[:n]) == "end" {
			break
		}

		// measure throughput
		bytes_received += float64(n)
		throughput_dict[bytes_received] = throughput_so_far(start, bytes_received)

		// send back a ping
		conn.Write([]byte("hello"))
	}
	ch <- true // can stop sending pings
	return throughput_dict
}

/*Very similar to start tcp, except sends back a packet with the rec. timestamp*/
func start_remy(wg sync.WaitGroup, start time.Time, ch chan<- bool) map[float64]float64 {
	defer wg.Done()
	throughput_dict := map[float64]float64{} // returns a map of bytes so far to throughput at that time
	bytes_received := float64(0)
	buf := make([]byte, 2048)

	// create connection
	conn, err := net.Dial("udp", config.SERVER_IP+":"+config.MEASURE_SERVER_PORT)
	CheckError(err)
	defer conn.Close() // close connection at end of function

	// send the start message, then wait for data
	conn.Write([]byte("remy"))

	// loop to read bytes and send back to the server
	for {
		n, err := conn.Read(buf)
		CheckError(err)

		// current ending condition to close the socket
		if string(buf[:n]) == "end" {
			break
		}

		// measure throughput
		bytes_received += float64(n)
		throughput_dict[bytes_received] = throughput_so_far(start, bytes_received)

		// echo packet with receive timestamp
		echo := SetHeaderVal(buf[:n], config.RECEIVE_TIMESTAMP_START, binary.LittleEndian, elapsed(start))
		conn.Write(echo.Bytes())
	}
	ch <- true // can stop sending pings
	return throughput_dict
}

/*Starts a ping chain - stops when the other goroutine sends a message over a channel*/
func ping_server(wg sync.WaitGroup, start time.Time, ch <-chan bool) map[float64]float64 {
	defer wg.Done()
	rtt_dict := map[float64]float64{}
	ping := []byte("ping")
	buf := make([]byte, 2048)
	// create tcp connection to the server
	conn, err := net.Dial("tcp", config.SERVER_IP+":"+config.PING_SERVER_PORT)
	CheckError(err)
	defer conn.Close()
	end := false
	for end {
		select {
		case <-ch:
			end = true
		default:
			send_timestamp := elapsed(start)
			conn.Write(ping)
			_, err := conn.Read(buf)
			rec_timestamp := elapsed(start)
			CheckError(err)
			rtt_dict[send_timestamp] = (rec_timestamp - send_timestamp)
		}
	}
	return rtt_dict
}

func run_experiment(f func(wg sync.WaitGroup, start time.Time, ch chan<- bool) map[float64]float64) CCResults {
	var wg sync.WaitGroup
	end_ping := make(chan bool)
	start := time.Now()
	throughput := map[float64]float64{}
	ping_results := map[float64]float64{}

	wg.Add(1)
	go func() {
		throughput = f(wg, start, end_ping)
	}()

	wg.Add(1)
	go func() {
		ping_results = ping_server(wg, start, end_ping)
	}()

	wg.Wait()

	// NOTE: this is bad go programming - you're supposed to send the return value from the go routine in a channel
	// but we're waiting till the threads finish anyway
	results := CCResults{Throughput: throughput, Ping: ping_results}
	return results
}

/*Encodes the inner CC results struct*/
func encodeCCResults(cc *CCResults) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(cc.Throughput)
	e.Encode(cc.Ping)
	return w.Bytes()
}

/*Not sure about marshalling in Go: I thought I would have to make the inner structs into bytes before encoding the outer struct*/
func encodeFinalResults(res *Results) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(res.Remy)
	e.Encode(res.TCP)
	return w.Bytes()
}

type CCResults struct {
	Throughput map[float64]float64
	Ping       map[float64]float64
}

type Results struct {
	Remy []byte // encoded remy results
	TCP  []byte // encoded TCP results
}

/*Client will do Remy experiment first, then Cubic experiment, then send data back to the server*/
func main() {
	remy := run_experiment(start_remy)
	tcp := run_experiment(start_tcp)

	// encode the results and send to the server
	results := Results{Remy: encodeCCResults(&remy), TCP: encodeCCResults(&tcp)}
	exp := encodeFinalResults(&results)

	// make TCP connection to send to the server
	conn, err := net.Dial("tcp", config.SERVER_IP+":"+config.MEASURE_SERVER_PORT)
	CheckError(err)
	defer conn.Close()
	conn.Write(exp)

}
