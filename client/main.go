// client file
/*
// basic idea for 3 functions: start remy, start_tcp, and ping
-> start remy sends the start remy message to the server and waits until the end message and counts throughput_dict
-> ping sends ping messages at the same start time
		*** could use channels to stop them
-> start tcp will work similarly
*/
package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"
)

var wg sync.WaitGroup

type TCPHeader struct {
	seq_num            int
	flow_id            int
	src_id             int
	sender_timestamp   float64
	receiver_timestamp float64
}

func CheckError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
	}
}

func elapsed(start time.Time) uint64 {
	return uint64(time.Since(start).Seconds() * 1000)
}

func start_remy(now time.Time) map[uint64]uint64 {
	defer wg.Done()
	throughput_dict := map[uint64]uint64{}
	bytes_so_far := uint64(0)

	p := make([]byte, 2048)
	conn, err := net.Dial("udp", "127.0.0.1:1234")
	defer conn.Close()
	CheckError(err)
	// send the start remy message
	fmt.Fprintf(conn, "start_remy")

	// now wait and read messages until the end messages
	for {
		_, err = bufio.NewReader(conn).Read(p)
		// end if the server sends "end"
		end_bytes := []byte("end") // NOTE: I couldn't get this stopping condition to work properly
		fmt.Printf("P is %s\n", string(p))
		if reflect.DeepEqual(end_bytes, p) {
			fmt.Println("In break")
			break
		}
		// send back the packet header
		if err == nil {
			fmt.Printf("%s\n", p)
		} else {
			fmt.Printf("Some error %v\n", err)
			break
		}
		bytes_so_far += 1500
		current_throughput := (bytes_so_far) / elapsed(now)
		fmt.Println("Bytes so far ", bytes_so_far, " current throughput ", current_throughput)
		throughput_dict[bytes_so_far] = current_throughput
		echo := make([]byte, 20)
		copy(p[0:20], echo)
		elapsed := elapsed(now)
		time_buf := make([]byte, 8)
		binary.PutUvarint(time_buf, elapsed)
		send := append(echo, time_buf...)
		fmt.Println("About to send more shit")
		conn.Write(send)
	}
	return throughput_dict

}

func sendPings(start time.Time) map[uint64]uint64 {
	// function that sends pings to our server - to be run in parallel with start_remy
	defer wg.Done()
	rtt_dict := map[uint64]uint64{}
	p := make([]byte, 2048)
	conn, err := net.Dial("udp", "127.0.0.1:1235")
	defer conn.Close()
	CheckError(err)

	// send pings and record RTTs
	// TODO: should be a loop but rn not yet
	send := elapsed(start)
	fmt.Fprintf(conn, "ping")
	_, err = bufio.NewReader(conn).Read(p)
	if err == nil {
		fmt.Printf("%s\n", p)
	} else {
		fmt.Printf("Some error %v\n", err)
	}
	received := elapsed(start)
	rtt := received - send
	rtt_dict[send] = rtt

	return rtt_dict
}

func start_tcp(start time.Time) map[uint64]uint64 {
	defer wg.Done()
	throughput_dict := map[uint64]uint64{}
	p := make([]byte, 2048)
	conn, err := net.Dial("tcp", "127.0.0.1:1235")
	defer conn.Close()
	fmt.Fprintf(conn, "start_tcp")
	CheckError(err)
	for {
		end_bytes := []byte("end")
		fmt.Printf("P is %s\n", string(p))
		if reflect.DeepEqual(end_bytes, p) {
			fmt.Println("In break")
			break
		}
		fmt.Fprintf(conn, "ping")
		_, err = bufio.NewReader(conn).Read(p)
		if err == nil {
			fmt.Printf("%s\n", p)
		} else {
			fmt.Printf("Some error %v\n", err)
		}
	}
	return throughput_dict

}

func main() {
	// run Remy and ping at same time,
	// then run TCP and ping at same time
	start := time.Now()
	wg.Add(1)
	go func() {
		dict := start_remy(start)
		println("Dict is", dict)
		for key, value := range dict {
			fmt.Println("Maps ", key, " to ", value)
		}
	}()

	wg.Add(1)
	go func() {
		rtt_dict := sendPings(start)
		for key, value := range rtt_dict {
			fmt.Println("rtt dict Maps ", key, " to ", value)
		}
	}()
	wg.Wait()

}
