package main

import (
	"../config"
	"../results"
	"bufio"
	"bytes"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var sendBuf []byte = make([]byte, config.TCP_TRANSFER_SIZE)
var rng *rand.Rand

/*****************************************************************************/
/*Taken from a stack overflow post for exec'ing commands in go with pipes*/
func Execute(output_buffer *bytes.Buffer, stack ...*exec.Cmd) (err error) {
	var error_buffer bytes.Buffer
	pipe_stack := make([]*io.PipeWriter, len(stack)-1)
	i := 0
	for ; i < len(stack)-1; i++ {
		stdin_pipe, stdout_pipe := io.Pipe()
		stack[i].Stdout = stdout_pipe
		stack[i].Stderr = &error_buffer
		stack[i+1].Stdin = stdin_pipe
		pipe_stack[i] = stdout_pipe
	}
	stack[i].Stdout = output_buffer
	stack[i].Stderr = &error_buffer

	if err := call(stack, pipe_stack); err != nil {
		log.Fatalln(string(error_buffer.Bytes()), err)
	}
	return err
}

func call(stack []*exec.Cmd, pipes []*io.PipeWriter) (err error) {
	if stack[0].Process == nil {
		if err = stack[0].Start(); err != nil {
			return err
		}
	}
	if len(stack) > 1 {
		if err = stack[1].Start(); err != nil {
			return err
		}
		defer func() {
			if err == nil {
				pipes[0].Close()
				err = call(stack[1:], pipes[1:])
			}
		}()
	}
	return stack[0].Wait()
}
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

			clientIPPort := conn.RemoteAddr().String()
			clientIP := strings.Split(clientIPPort, ":")[0] // of form IP:port
			log.WithFields(log.Fields{"client ip": clientIP}).Info("client IP")
			lossRate, delayMap := runGCC(srcport, clientIP, alg)
			log.Info("Delay map: ", delayMap)
			// can close TCP connection after writing the loss rate back
			log.Info("Loss rate: ", lossRate)
			info := results.LossRTTInfo{LossRate: lossRate, Delay: delayMap}
			conn.Write(results.EncodeLossRTTInfo(&info))
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
func currentTime() string {
	hour, min, sec := time.Now().Clock()
	return fmt.Sprintf("%d.%d.%d", hour, min, sec)
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

func srttInfoServer() {
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.SRTT_INFO_PORT)
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
		go handleSRTTRequest(conn)
	}
}
func handleSRTTRequest(conn *net.TCPConn) {
	defer conn.Close()
	clientIPPort := conn.RemoteAddr().String()
	clientIP := strings.Split(clientIPPort, ":")[0]
	reqBuf := make([]byte, config.MAX_REQ_SIZE)
	n, err := conn.Read(reqBuf)
	timePort := string(reqBuf[:n])
	curTime := strings.Split(timePort, "->")[0]
	clientPort := strings.Split(timePort, "->")[1]

	// filename = IP_time_tcpprobe.log
	tcpprobeInfo := fmt.Sprintf("%s_%s_tcpprobe.log", clientIP, curTime)

	// manually PARSE the file for the srtt info and get an array of RTTs
	// open the tcpprobe file line by line and read it
	open := false
	tries := 0
	for !(open) && tries < 5 {
		if _, err := os.Stat(tcpprobeInfo); os.IsNotExist(err) {
			tries++
			time.Sleep(time.Second * 2)
		} else {
			open = true
		}
	}
	emptyRet := results.LossRTTInfo{Delay: results.TimeRTTMap{}, LossRate: 0}
	if tries >= 5 {
		conn.Write(results.EncodeLossRTTInfo(&emptyRet))
		return
	}
	probeFile, err := os.Open(tcpprobeInfo)
	if err != nil {
		log.Warn("err on opening file: ", err)
		conn.Write(results.EncodeLossRTTInfo(&emptyRet))
		return
	}
	defer probeFile.Close()
	scanner := bufio.NewScanner(probeFile)
	rttDict := make(map[float32]float32)
	firstTimestamp := float64(0)
	for scanner.Scan() {
		line := scanner.Text()
		brokenLine := strings.Split(line, " ")
		if !(strings.Contains(line, "ffff")) { // to be able to parse for mahimahi correctly
			continue
		}
		// check client port is in the RCV field, as it could show up in the timestamp field
		rcvField := brokenLine[2]
		if !(strings.Contains(rcvField, clientPort)) {
			continue
		}
		timestamp, _ := strconv.ParseFloat(brokenLine[0], 64)
		srtt, _ := strconv.ParseFloat(brokenLine[9], 64)
		if firstTimestamp == float64(0) {
			firstTimestamp = timestamp
		}
		//log.WithFields(log.Fields{"first": firstTimestamp, "srtt": srtt / 1000, "timestamp": (timestamp - firstTimestamp)}).Info("stuff")
		rttDict[float32((timestamp-firstTimestamp)*1000)] = float32(srtt / 1000)
	}

	// need to send back this rttDict
	lossRTTInfo := results.LossRTTInfo{LossRate: 0, Delay: rttDict}
	conn.Write(results.EncodeLossRTTInfo(&lossRTTInfo))
	// then delete the file
	os.Remove(tcpprobeInfo)
	return
}
func handleRequestTCP(conn *net.TCPConn) {
	clientIPPort := conn.RemoteAddr().String()
	clientIP := strings.Split(clientIPPort, ":")[0]
	clientPort := strings.Split(clientIPPort, ":")[1]
	log.Info("Client port is: ", clientIPPort)
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

	reqTime := string(reqBuf[:n])
	req := strings.Split(reqTime, "->")[0]
	curTime := strings.Split(reqTime, "->")[1]

	file, err := conn.File()
	if err != nil {
		log.Error(err)
	}

	var ccname string
	if req[:3] == "ccp" {
		ccname = "ccp"
		// sp := strings.Split(req, "-")
		// ccp_alg := sp[1]
		// TODO assuming nimbus for now
		// ccp_params := strings.Split(sp[2], ",")
		// TODO need to pass these params to ccp
	} else {
		ccname = req
	}
	syscall.SetsockoptString(int(file.Fd()), syscall.IPPROTO_TCP, config.TCP_CONGESTION, ccname)

	on_time := time.Millisecond * config.MEAN_ON_TIME_MS
	conn.Write(startBuf)
	buf := make([]byte, config.ACK_LEN)
	conn.Read(buf) // wait for ack back
	log.Info("Connection established, starting sendloop")
	on_timer := time.After(on_time)
sendloop:
	for {
		select {
		case <-on_timer:
			log.Info("closing?")
			break sendloop
		default:
			//log.Warn("Waiting to write to TCP connection")
			conn.Write(sendBuf)
		}
	}
	log.Info("Done. Closing connection...")
	// now run grep command to get all the SRTT estimates and output it to a file
	//curTime := currentTime()
	parseString := clientPort
	// NOTE: for mahimahi, grepping for client port will result in lines both from server -> NAT and NAT -> client
	// we would want NAT -> client lines -> so hack, just check for "ffff"
	var b bytes.Buffer
	if err := Execute(&b,
		exec.Command("/bin/cat", config.SERVER_TCPPROBE_OUTPUT),
		exec.Command("/bin/grep", parseString),
	); err != nil {
		log.Fatal(err)
	}
	tcpprobeInfo := fmt.Sprintf("%s_%s_tcpprobe.log", clientIP, curTime)
	err = ioutil.WriteFile(tcpprobeInfo, b.Bytes(), 0644)

	if err != nil {
		log.Warn(err)
	}
	return
}

func runGCC(srcport string, ip string, alg string) (float64, results.TimeRTTMap) {
	log.Info(alg)
	udp_alg := "remy"
	alg_path := strings.Split(alg, "->")[0]
	port := strings.Split(alg, "->")[1]                           // assume "remy=pathname=TRAIN_LENGTH=LINKRATE->clientport"
	path := config.PATH_TO_RATS + strings.Split(alg_path, "=")[1] // assume alg is remy-ratname at hardcoded path
	trainLength := strconv.Itoa(1)
	linkRate := strconv.Itoa(1)
	log.Info(port)
	on_time := strconv.Itoa(config.MEAN_ON_TIME_MS)
	off_time := strconv.Itoa(0)   // have a 0 off time
	num_cycles := strconv.Itoa(1) // send for 1 on and off period
	log.WithFields(log.Fields{"num cycles": num_cycles}).Info("num cycles")
	currentTime := currentTime()
	logfileName := fmt.Sprintf("%s_%s.log", ip, currentTime)
	lossRate := float64(0)
	if len(strings.Split(alg_path, "=")) == 4 {
		// train length and linkspeed provided
		trainLength = strings.Split(alg_path, "=")[2]
		linkRate = strings.Split(alg_path, "=")[3]
		linkRateMbps, err := strconv.Atoi(linkRate)
		if err != nil {
			log.WithFields(log.Fields{"link rate": linkRate}).Warn("Error when converting linkRate to right units: ", err)
		}
		linkRatePPS := linkRateMbps * 125000 / config.PING_SIZE_BYTES
		linkRate = strconv.Itoa(linkRatePPS)
	}
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
			"train_length=" + trainLength,
			"linkrate=" + linkRate,
			"traffic_params=deterministic,num_cycles=" + num_cycles,
			"if=" + path,
			"logfile=" + logfileName,
		}
		// TODO remove stdout when done testing
		cmd := exec.Command(config.PATH_TO_GENERIC_CC, args...)
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			log.Info("std out error from running generic CC ermagerd")
			log.Error(err)
		}
		log.Info("Done with genericCC program")
		logfile, err := os.Open(logfileName)
		if err != nil {
			log.Warn(err)
			return lossRate, results.TimeRTTMap{}
		}
		defer func() {
			logfile.Close()
			if _, err := os.Stat(logfileName); !os.IsNotExist(err) {
				os.Remove(logfileName)
			}
		}()

		scanner := bufio.NewScanner(logfile)
		lossRate := float64(0)
		lossData := make([]float64, 0)
		rttDict := results.TimeRTTMap{}
		lines := make([]string, 0)
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
		}
		for _, line := range lines {
			log.Info(line)
			broken := strings.Split(line, ",")
			if len(broken) == 2 {
				sendtime, _ := strconv.ParseFloat(strings.Split(line, ",")[0], 64)
				rtt, _ := strconv.ParseFloat(strings.Split(line, ",")[1], 64)
				rttDict[float32(sendtime)] = float32(rtt)
			} else {
				lossRate, _ := strconv.ParseFloat(line, 64)
				lossData = append(lossData, lossRate)
			}
		}
		lossRate = lossData[0]

		if err := scanner.Err(); err != nil {
			log.Warn(err)
			return lossRate, rttDict
		}
		log.Info("Finished handling request UDP, lossRATE: ", lossRate)
		return lossRate, rttDict
	default:
		log.WithFields(log.Fields{"alg": alg}).Error("udp algorithm not implemented")
	}
	log.Info("Finished handling request UDP, lossRATE: ", lossRate)
	return lossRate, results.TimeRTTMap{}
}

func main() {
	quit := make(chan struct{})

	go pingServerUDP()    // UDP pings
	go pingServerTCP()    // TCP pings
	go measureServerTCP() // Measure TCP throughput
	go openUDPServer()    // open port to measure UDP throughput
	go srttInfoServer()   // read the tcp probe output files, parse srtt array, and delete logfiles

	<-quit
}
