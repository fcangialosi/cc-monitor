package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"../config"
	"../results"
	"github.com/rdegges/go-ipify"
	log "github.com/sirupsen/logrus"
)

var sendBuf []byte = make([]byte, config.TCP_TRANSFER_SIZE)
var rng *rand.Rand

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func checkErrMsg(err error, message string) {
	if err != nil {
		log.WithFields(log.Fields{"msg": message}).Fatal(err)
	}
}

/*Handles the "open genericCC protocol" -> over TCP*/
func measureServerUDP() {
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

	log.WithFields(log.Fields{"client": clientIPPort}).Info("New measurement requested")

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

	log.WithFields(log.Fields{"req": string(reqBuf)}).Info("Read from client")

	reqTime := strings.SplitN(string(reqBuf[:n]), " ", 3)
	curTime := reqTime[0]
	alg := reqTime[1]
	params := reqTime[2]
	parsed_params := parseAlgParams(params)

	log.WithFields(log.Fields{"curTime": curTime, "alg": alg, "params": params, "parsed_params": parsed_params}).Info("Parsed client req")

	on_time := time.Millisecond * config.MEAN_ON_TIME_MS
	if manual_exp_time, ok := parsed_params["exp_time"]; ok {
		new_on_time, err := time.ParseDuration(manual_exp_time)
		if err != nil {
			log.Warn("unable to parse experiment time from params: ", manual_exp_time)
		} else {
			on_time = new_on_time
		}
	}

	log.WithFields(log.Fields{"on_time": on_time}).Info("Starting experiment")

	file, err := conn.File()
	if err != nil {
		log.Error(err)
	}

	var ccname string
	// Start CCP process in the background
	if alg[:3] == "ccp" {
		ccname = "ccp"
		logname := fmt.Sprintf("%s_%s.log", alg, strings.Replace(params, " ", "_", -1))
		args := []string{
			config.PATH_TO_CCP,
			"--datapath=kernel",
			"--congAlg=" + alg[4:],
			"--logfile=" + logname,
		}
		args_string := strings.Join(args, " ") + " " + params
		log.WithFields(log.Fields{"args": args, "logname": logname, "args_string": args_string}).Info("exec")
		// cmd := shellCommand(args_string, false)
		cmd := exec.Command("sudo", strings.Split(args_string, " ")...)
		if err := cmd.Start(); err != nil {
			log.WithFields(log.Fields{"err": err, "cmd": cmd}).Error("Error starting ccpl")
		}
		log.Info("Command started.")

		defer func() {
			log.Info("Running from defer")
			if err := cmd.Process.Kill(); err != nil {
				log.Warn("error stopping ccpl")
			}
			log.Info("Proc killed")
			// Copy logfile to database
			remotepath := config.DB_SERVER_CCP_TMP + my_public_ip + "-" + strings.Split(conn.RemoteAddr().String(), ":")[0] + "/"
			log.WithFields(log.Fields{"remotepath": remotepath}).Info("Copying to db")

			shellCommand(fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no %s mkdir -p %s", config.PATH_TO_PRIV_KEY, config.DB_SERVER, remotepath), true)
			shellCommand(fmt.Sprintf("scp -i %s %s %s:%s", config.PATH_TO_PRIV_KEY, logname, config.DB_SERVER, remotepath), true)

			log.Info("done defer")

		}()

		// Give ccpl some time to startup
		time.Sleep(1 * time.Second)

		log.Info("Done waiting for CCPL")

	} else {
		ccname = alg
	}
	log.Info("setting sockopt to ", ccname)
	syscall.SetsockoptString(int(file.Fd()), syscall.IPPROTO_TCP, config.TCP_CONGESTION, ccname)

	log.Info("sending ack")
	conn.Write(startBuf)
	buf := make([]byte, config.ACK_LEN)
	conn.Read(buf) // wait for ack back
	log.Info("Connection established, starting sendloop")

	// NOTE: for mahimahi, grepping for client port will result in lines both from server -> NAT and NAT -> client
	// we would want NAT -> client lines -> so hack, just check for "ffff"
	parseString := clientPort
	probeLog := fmt.Sprintf("%s_%s_tcpprobe.log", clientIP, curTime)
	probe := shellCommand(fmt.Sprintf("\"cat /proc/net/tcpprobe | grep %s > %s\"", parseString, probeLog), false)

	on_timer := time.After(on_time)
sendloop:
	for {
		select {
		case <-on_timer:
			log.Info("Finished")
			break sendloop
		default:
			//log.Warn("Waiting to write to TCP connection")
			conn.Write(sendBuf)
		}
	}
	log.Info("Done. Closing connection...")
	if err := probe.Process.Kill(); err != nil {
		log.Warn("error stopping probe")
	}
	log.Info("Probe killed")

	return
}

func parseAlgParams(line string) (params map[string]string) {
	params = make(map[string]string)
	sp := strings.Split(line, " ")
	for _, param := range sp {
		kv := strings.Split(param, "=")
		params[kv[0]] = kv[1]
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

func shellCommand(cmd string, wait bool) *exec.Cmd {
	proc := exec.Command("/bin/bash", "-c", cmd)
	if wait {
		if err := proc.Run(); err != nil {
			log.WithFields(log.Fields{"err": err, "cmd": cmd}).Error("Error running shell command")
		}
	} else {
		if err := proc.Start(); err != nil {
			log.WithFields(log.Fields{"err": err, "cmd": cmd}).Error("Error starting shell command")
		}
	}
	return proc
}

var my_public_ip string

func main() {
	quit := make(chan struct{})

	log.Info("Preparing TCP Probe")
	shellCommand("sudo modprobe tcp_probe port="+config.MEASURE_SERVER_PORT+" full=1", true)
	shellCommand("sudo chmod 444 /proc/net/tcpprobe", true)

	public_ip, err := ipify.GetIp()
	if err != nil {
		log.Error("error finding public IP: ", err)
	}
	my_public_ip = public_ip
	log.Info("Found my public IP")
	log.Info(my_public_ip)

	go measureServerTCP() // Measure TCP throughput
	go measureServerUDP() // open port to measure UDP throughput
	go srttInfoServer()   // read the tcp probe output files, parse srtt array, and delete logfiles

	<-quit
}
