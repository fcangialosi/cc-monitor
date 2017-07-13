package main

import (
	"net"
	"syscall"
	"time"

	"../config"

	log "github.com/sirupsen/logrus"
)

var sendbuf []byte = make([]byte, config.SEND_BUF_BYTES)

/*****************************************************************************/

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

func handleRequest(conn *net.TCPConn) {
	defer conn.Close()

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

func dbServer() {
	// TODO handle incoming reports, send them on channel to db worker
}

func dbWorker() {
	// TODO reads jobs from channel, writes them to db
}

func main() {
	quit := make(chan struct{})

	go pingServer()
	go measureServer()
	go dbServer()
	go dbWorker()

	<-quit
}
