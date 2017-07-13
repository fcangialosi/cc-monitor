package main

import (
	"fmt"
	"net"
	"syscall"
	"time"

	"../config"

	log "github.com/sirupsen/logrus"
)

var sendbuf []byte = make([]byte, config.SEND_BUF_BYTES)

/*****************************************************************************/

func listen_for_Remy() {
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
			//sendResponse(ser, remoteaddr, "Sup client")
		} else {
			//sendResponse(ser, remoteaddr, "end")
		}

	}
}

func handleRequest(conn *net.TCPConn) {
	reqBuf := make([]byte, config.MAX_REQ_SIZE)
	n, err := conn.Read(reqBuf)
	if err != nil {
		log.Error(err)
	}
	if n <= 0 {
		log.Error("read 0 bytes from client")
		return
	}

	req := string(reqBuf[:n])
	log.Info("starting measurement", log.Fields{"alg": req})

	switch req {
	case "remy": // handle remy
		// TODO launch genericc
	default: // must be one of the tcps
		// set cc alg
		file, err := conn.File()
		if err != nil {
			log.Error(err)
		}
		syscall.SetsockoptString(int(file.Fd()), syscall.IPPROTO_TCP, config.TCP_CONGESTION, req)

		// generate on/off distributions
		prng := getNewPRNG()
		on_dist := createExpDist(config.MEAN_ON_TIME_MS, prng)
		off_dist := createExpDist(config.MEAN_OFF_TIME_MS, prng)

		for i := 0; i < config.NUM_CYCLES; i++ {
			// on
			select {
			case <-time.After(time.Millisecond * time.Duration(on_dist.Sample())):
				break
			default:
				conn.Write(sendbuf)
			}

			// off
			<-time.After(time.Millisecond * time.Duration(off_dist.Sample()))
		}
	}

}

func measureServer() {
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.MEASURE_SERVER_PORT)
	if err != nil {
		log.Fatal(err)
	}

	server, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := server.AcceptTCP()
		if err != nil {
			log.Warning(err)
		}
		go handleRequest(conn)
	}
}

func pingServer() {
	p := make([]byte, config.PING_SIZE_BYTES)

	laddr, err := net.ResolveUDPAddr("udp", ":"+config.PING_SERVER_PORT)
	if err != nil {
		log.Fatal(err)
	}
	server, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		_, raddr, err := server.ReadFromUDP(p)
		if err != nil {
			log.Fatal(err)
		}
		server.WriteToUDP(p, raddr)
	}

}

func main() {
	quit := make(chan struct{})

	go pingServer()
	//go measureServer()
	//go dbServer(): handle incoming reports, send them on channel to db worker
	//go dbWorker(): reads jobs from channel, writes them to db

	<-quit
}
