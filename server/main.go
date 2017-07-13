// server file
// works with the functions in the other file
package main

import (
	"fmt"
	"net"
	"os"
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

/* A Simple function to verify error */
func CheckError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(0)
	}
}

func sendResponse(conn *net.UDPConn, addr *net.UDPAddr, message string) {
	fmt.Println("Sending ", message)
	_, err := conn.WriteToUDP([]byte(message), addr)
	if err != nil {
		fmt.Printf("Couldn't send response %v", err)
	}
}

func listen_for_Remy() {
	defer wg.Done()
	p := make([]byte, 2048)
	addr := net.UDPAddr{
		Port: 1234,
		IP:   net.ParseIP("127.0.0.1"),
	}
	ser, err := net.ListenUDP("udp", &addr)
	if err != nil {
		fmt.Printf("Some error %v\n", err)
		return
	}
	_, remoteaddr, err := ser.ReadFromUDP(p)
	for i := 1; i <= 10; i++ {
		fmt.Printf("Read a message from %v %s \n", remoteaddr, p)
		if err != nil {
			fmt.Printf("Some error  %v", err)
			continue
		}
		if i < 9 {
			sendResponse(ser, remoteaddr, "Sup client")
		} else {
			sendResponse(ser, remoteaddr, "end")
		}

	}
}

func listen_for_pings() {
	defer wg.Done()
	p := make([]byte, 2048)
	addr := net.UDPAddr{
		Port: 1235,
		IP:   net.ParseIP("127.0.0.1"),
	}

	ser, err := net.ListenUDP("udp", &addr)
	CheckError(err)
	_, remoteaddr, err := ser.ReadFromUDP(p)
	time.Sleep(10)
	sendResponse(ser, remoteaddr, "ping")

}

func main() {
	wg.Add(1)
	go listen_for_Remy()
	wg.Add(1)
	go listen_for_pings()
	wg.Wait()
}
