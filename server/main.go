package main

import (
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"../config"
	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

var sendBuf []byte = make([]byte, config.TCP_TRANSFER_SIZE)
var rng *rand.Rand

/*****************************************************************************/

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func checkErrMsg(err error, message string) {
	if err != nil {
		log.WithFields(log.Fields{"msg": message}).Fatal(err)
	}
}

/*Handles the "open genericCC protocol" -> over TCP*/
func openUDPServer() {
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.OPEN_UDP_PORT)
	checkErrMsg(err, "open TCP port for starting genericCC")

	server, err := net.ListenTCP("tcp", laddr)
	checkErrMsg(err, "listen TCP func open genericCC function")

	for {
		conn, err := server.AcceptTCP()
		if err != nil {
			log.Warning(err)
			continue
		}

		go func(c *net.TCPConn) {
			defer conn.Close()
			reqbuf := make([]byte, config.MAX_REQ_SIZE)
			n, err := conn.Read(reqbuf)
			if n <= 0 {
				log.Error("Read 0 bytes from the client")
				return
			}
			if err != nil {
				log.Warn(err)
			}

			alg := string(reqbuf[:n])

			// for right now -> alg must be remy
			// alg must have remy in it
			if alg[:4] != config.REMY {
				log.WithFields(log.Fields{"protocol": string(reqbuf[:n])}).Info("Read non-remy alg from client on open UDP port")
			}

			// pick a port for genericCC to run
			srcport := strconv.Itoa(rng.Intn(config.MAX_PORT-1025) + 1025)
			// SYN-ACK
			conn.Write([]byte(srcport)) // TCP handles reliability

			// wait for ACK from client
			n, err = conn.Read(reqbuf)
			if err != nil {
				log.Warn(err)
			}
			if string(reqbuf[:n]) != config.ACK {
				log.Warn("Did not receive ack after sending client src port")
				return
			} else {
				log.Info("Read ack")
			}
			clientIPPort := conn.RemoteAddr().String()
			clientIP := strings.Split(clientIPPort, ":")[0] // of form IP:port
			log.WithFields(log.Fields{"client ip": clientIP}).Info("client IP")
			runGCC(srcport, clientIP, alg)
			// can close TCP connection
		}(conn)

	}
}

func pingServerTCP() {
	p := []byte("ACK")
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
			defer c.Close()
			// loop of reading and writing until eof
			recv := make([]byte, config.PING_SIZE_BYTES)
			for {
				n, err := c.Read(recv)
				if err == io.EOF {
					return // client disconnected
				}
				//log.WithFields(log.Fields{"i": string(recv[:n])}).Info("got ping")
				c.Write(recv[:n])
			}
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
			//log.WithFields(log.Fields{"IP": addr.IP, "Port": addr.Port}).Info("Writing back ping channel")
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

		on_time := time.Millisecond * time.Duration(on_dist.Sample()+config.MEAN_ON_TIME_MS)
		// on - send start flow message
		log.WithFields(log.Fields{"on": on_time}).Info("new on for tcp")
		conn.Write(startBuf)
		log.Info("Wrote to start buf")
		buf := make([]byte, config.ACK_LEN)
		conn.Read(buf) // wait for ack back
		log.Info("Got ack")
		on_timer := time.After(on_time)
	sendloop:
		for {
			select {
			case <-on_timer:
				break sendloop
			default:
				//log.Warn("Waiting to write to TCP connection")
				conn.Write(sendBuf)
			}
		}
		log.Info("Done with on - about to close connection")
		err = conn.Close()
	}
	return
}

func runGCC(srcport string, ip string, alg string) {
	log.Info(alg)
	udp_alg := "remy"
	alg_path := strings.Split(alg, "->")[0]
	port := strings.Split(alg, "->")[1]                           // assume "remy=pathname->clientport"
	path := config.PATH_TO_RATS + strings.Split(alg_path, "=")[1] // assume alg is remy-ratname at hardcoded path
	log.Info(port)
	on_time := strconv.Itoa(config.MEAN_ON_TIME_MS)
	off_time := strconv.Itoa(0)   // have a 0 off time
	num_cycles := strconv.Itoa(1) // send for 1 on and off period
	log.WithFields(log.Fields{"num cycles": num_cycles}).Info("num cycles")

	// Now we can start genericCC
	// set MIN_RTT env variable
	os.Setenv("MIN_RTT", "150")
	switch udp_alg {
	case "remy":
		args := []string{
			"serverip=" + ip,
			"serverport=" + port,
			"sourceport=" + srcport,
			"onduration=" + on_time,
			"offduration=" + off_time,
			"cctype=remy",
			"traffic_params=exponential,num_cycles=" + num_cycles,
			"if=" + path,
		}
		// TODO remove stdout when done testing
		cmd := exec.Command(config.PATH_TO_GENERIC_CC, args...)
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			log.Info("std out error from running generic CC ermagerd")
			log.Error(err)
		}
		log.Info("Done with genericCC program")
		// server.WriteToUDP([]byte(config.FIN), raddr)
	default:
		log.WithFields(log.Fields{"alg": alg}).Error("udp algorithm not implemented")
	}
	log.Info("Finished handling request UDP")
}

func main() {
	quit := make(chan struct{})

	go pingServerUDP()    // UDP pings
	go pingServerTCP()    // TCP pings
	go measureServerTCP() // Measure TCP throughput
	go openUDPServer()    // open port to measure UDP throughput

	<-quit
}
