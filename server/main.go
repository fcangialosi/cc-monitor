package main

import (
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"../config"
	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

var sendBuf []byte = make([]byte, config.TRANSFER_BUF_SIZE)
var rng *rand.Rand

/*****************************************************************************/

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}
func pingServerTCP() {
	p := make([]byte, config.PING_SIZE_BYTES)
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.PING_TCP_SERVER_PORT)
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
		go func(c *net.TCPConn, buf []byte) {
			defer conn.Close()
			conn.Write(p)
		}(conn, p)
	}
}

func pingServerUDP() {
	p := make([]byte, config.PING_SIZE_BYTES)

	laddr, err := net.ResolveUDPAddr("udp", ":"+config.PING_UDP_SERVER_PORT)
	if err != nil {
		log.Fatal(err)
	}
	server, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		//log.Warn("reading from ping server")
		_, raddr, err := server.ReadFromUDP(p)
		if err != nil {
			log.Fatal(err)
		}
		go func(s *net.UDPConn, addr *net.UDPAddr, buf []byte) {
			log.WithFields(log.Fields{"IP": addr.IP, "Port": addr.Port}).Info("Writing back ping channel")
			s.WriteToUDP(buf, addr)
		}(server, raddr, p)
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
	startBuf := []byte("START_FLOW")
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
			// on - send start flow message
			log.WithFields(log.Fields{"on": on_time}).Info("new on for tcp")
			conn.Write(startBuf)
		sendloop:
			for {
				select {
				case <-on_timer:
					break sendloop
				default:
					conn.Write(sendBuf)
				}
			}

			off_time := time.Millisecond * time.Duration(off_dist.Sample())
			log.WithFields(log.Fields{"off": off_time}).Info("new off for tcp")
			<-time.After(off_time)
		}
	}
	conn.Write([]byte(config.FIN))
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
		go handleRequestUDP(string(reqBuf[:n]), server, raddr)
	}
}

func handleRequestUDP(alg string, server *net.UDPConn, raddr *net.UDPAddr) {
	os.Setenv("MIN_RTT", "150")
	ip := raddr.IP.String()
	port := strconv.Itoa(raddr.Port)
	on_time := strconv.Itoa(config.MEAN_ON_TIME_MS)
	off_time := strconv.Itoa(config.MEAN_OFF_TIME_MS)
	num_cycles := strconv.Itoa(config.NUM_CYCLES)

	srcport := strconv.Itoa(rng.Intn(config.MAX_PORT-1025) + 1025)
	// SYN-ACK
	server.WriteToUDP([]byte(srcport), raddr)

	// Now we can start genericCC
	// set MIN_RTT env variable
	switch alg {
	case "remy":
		args := []string{
			"serverip=" + ip,
			"serverport=" + port,
			"sourceport=" + srcport,
			"onduration=" + on_time,
			"offduration=" + off_time,
			"cctype=remy",
			"traffic_params=exponential,num_cycles=" + num_cycles,
			"if=" + config.PATH_TO_REMY_CC,
		}
		// TODO remove stdout when done testing
		cmd := exec.Command(config.PATH_TO_GENERIC_CC, args...)
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			log.Error(err)
		}
		// server.WriteToUDP([]byte(config.FIN), raddr)
	default:
		log.WithFields(log.Fields{"alg": alg}).Error("udp algorithm not implemented")
	}
	log.Info("Finished handling request UDP")
}

func main() {
	quit := make(chan struct{})

	go pingServerUDP()
	go pingServerTCP()
	go measureServerTCP()
	go measureServerUDP()

	<-quit
}
