package main

import (
	"net"
	"os/exec"
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

func measureServerTCP() {
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
		go handleRequestTCP(conn)
	}
}

func handleRequestTCP(conn *net.TCPConn) {
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

	switch req {
	case "remy": // handle remy
		// TODO launch genericc
	default: // must be one of the tcps
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
			on_time := time.Millisecond * time.Duration(on_dist.Sample())
			on_timer := time.After(on_time)
			// on
		sendloop:
			for {
				select {
				case <-on_timer:
					break sendloop
				default:
					conn.Write(sendbuf)
				}
			}

			off_time := time.Millisecond * time.Duration(off_dist.Sample())
			<-time.After(off_time)
		}
	}

}

func measureServerUDP() {
	laddr, err := net.ResolveUDPAddr("udp", ":"+config.MEASURE_SERVER_PORT)
	if err != nil {
		log.Fatal(err)
	}

	server, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	reqBuf := make([]byte, config.MAX_REQ_SIZE)
	for {
		n, raddr, err := server.ReadFromUDP(reqBuf)
		if err != nil {
			log.Fatal(err)
		}
		go handleRequestUDP(string(reqBuf[:n]), raddr)
	}
}

func handleRequestUDP(alg string, raddr *net.UDPAddr) {
	ip := raddr.IP.String()
	port := string(raddr.Port)

	switch alg {
	case "remy":
		args := []string{
			"serverip=" + ip,
			"serverport=" + port,
			"onduration=" + string(config.MEAN_ON_TIME_MS),
			"offduration=" + string(config.MEAN_OFF_TIME_MS),
			"cctype=remy",
			"traffic_params=exponential",
			"if=" + config.PATH_TO_REMY_CC,
		}
		if err := exec.Command(config.PATH_TO_GENERIC_CC, args...); err != nil {
			log.Error(err)
		}
	default:
		log.WithFields(log.Fields{"alg": alg}).Error("udp algorithm not implemented")
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
	go measureServerTCP()
	go measureServerUDP()
	go dbServer()
	go dbWorker()

	<-quit
}
