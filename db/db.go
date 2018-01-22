package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"cc-monitor/config"
	"cc-monitor/results"
	"cc-monitor/shared"

	log "github.com/sirupsen/logrus"
)

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

func getIPLocation(ip_file string, input_ip string) string {
	file, err := os.Open(ip_file)
	defer file.Close()
	checkError(err)
	scanner := bufio.NewScanner(file)
	line_num := 0
	for scanner.Scan() {
		if line_num != 0 {
			ip_line := scanner.Text() // line: IP UDP alg1 alg2 TCP alg1 alg2 ....
			ip_line_split := strings.Split(ip_line, " ")
			ip := ip_line_split[0]
			location := ip_line_split[1]
			if ip == input_ip {
				return location
			}
		}
		line_num++
	}
	checkError(scanner.Err())
	return "NOT_FOUND"
}

func introServer(ip_file string) {

	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.IP_SERVER_PORT())
	if err != nil {
		log.Fatal(err)
	}

	server, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := server.AcceptTCP()
		checkError(err)
		go func(c *net.TCPConn) {
			defer conn.Close()
			config := shared.ReadYAMLConfig(ip_file)
			log.Info("Sending config to client")
			conn.Write(shared.EncodeConfig(config))
		}(conn)
	}

}
func dbServer(ch chan results.CCResults) {
	/*
		Listens for TCP connections where client will send the report of the experiment over.
	*/
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.DB_SERVER_PORT())
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
		go func(c *net.TCPConn) {
			defer c.Close()
			p := make([]byte, 15000) // large buf size
			report_bytes := make([]byte, 0)
			// read until client sends EOF
			for {
				n, err := conn.Read(p)
				if err == io.EOF {
					log.Warn("client left")
					break
				}
				report_bytes = append(report_bytes, p[:n]...)
			}
			report := results.DecodeCCResults(report_bytes)
			report.ClientIP = strings.Split(conn.RemoteAddr().String(), ":")[0] // report client's public IP
			ch <- report
		}(conn)
	}

}

func dbWorker(ch chan results.CCResults, ip_file string) {
	/*
		Waits for jobs on the results channel -> and runs graphing and parsing scripts on them.
	*/
	for {
		report := <-ch
		// spawn go routine to deal with this report
		go func(rep results.CCResults) {
			// 1: create path where the graphs and bytes will go
			folder := shared.CreateLogFolder(rep.ClientIP, rep.ServerIP, rep.SendTime)
			// create individual folders for each algorithm to house the trials and logs
			for alg, _ := range rep.Throughput {
				shared.CreateLogAlgFolder(folder, alg)
			}

			// 2: write all the bytes to a file called {folder}/bytes.log
			bytesLogFile := shared.WriteBytes(results.EncodeCCResults(&rep), fmt.Sprintf("%s/%s", folder, "bytes.log"))

			// 3: transfer the ccp/tcpprobe logs for this client into the correct location
			ccpLogLocation := shared.CCPLogLocation(rep.ClientIP, rep.ServerIP)
			mvCommand := fmt.Sprintf("mv %s/* %s/", ccpLogLocation, folder)
			shellCommand(mvCommand, true)

			// 4: create the graphs using the graphing scripts

			// (a) Summary graph comparing bytes transferred of all protocols
			graphTitle, graphFilename := shared.CompareGraphName(rep.ClientIP, rep.ServerIP, rep.SendTime)
			graphScriptArgs := []string{bytesLogFile, graphFilename, graphTitle, folder}
			log.WithFields(log.Fields{"arg2": graphFilename}).Info("args to the main script")
			cmd := exec.Command(config.PATH_TO_GRAPH_SCRIPT, graphScriptArgs...) // graphing scripts  moves the image to path specified
			cmd.Stdout = os.Stdout
			if err := cmd.Run(); err != nil {
				log.Info("Error in running graphing script")
				log.Error(err)
			}

			// (b) Throughput/delay plots per algorithm - also does individual throughput delay plots
			for alg, _ := range rep.Throughput {
				alg = shared.RemoveSpacesAlg(alg)
				numFlows := fmt.Sprintf("%d", (len(rep.Throughput[alg]) - 1))
				algFolder := shared.SpecificAlgLogFolder(folder, alg)
				// log for graphing script
				throughputGraphTitle, throughputLogfile, throughputTmp := shared.ThroughputGraphName(graphFilename, alg)
				throughoutGraphArgs := []string{throughputLogfile, throughputTmp, throughputGraphTitle, folder, algFolder, numFlows}
				log.WithFields(log.Fields{"arg1": throughputLogfile, "arg2": throughputTmp}).Info("Arguments to throughput graph function")
				cmd = exec.Command(config.PATH_TO_GRAPH_THROUGHPUT_SCRIPT, throughoutGraphArgs...)
				cmd.Stdout = os.Stdout
				if err := cmd.Run(); err != nil {
					log.Info("Error in running graphing script for throughput graphs")
					log.Error(err)
				}
			}
		}(report)

	}
}

func getGraphInfo(ip_file string) {
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.DB_GRAPH_PORT())
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
		go func(c *net.TCPConn) {
			// decode the graph info and client IP, and from that construct the graph:w
			defer c.Close()
			p := make([]byte, 2048) // large buf size
			n, err := conn.Read(p)
			checkErrMsg(err, "reading URL prefix string")
			report := results.DecodeGraphInfo(p[:n])
			server_ip := report.ServerIP
			client_ip := strings.Split(c.RemoteAddr().String(), ":")[0]
			current_time := report.SendTime
			path := fmt.Sprintf("%s-%s/%s", shared.MachineHostname(server_ip), shared.MachineHostname(client_ip), current_time)
			// find the correct URL and return
			URL := config.URL_PREFIX + "/" + path
			conn.Write([]byte(URL))
		}(conn)
	}
}

func currentDate() string {
	_, month, day := time.Now().UTC().Date()
	return fmt.Sprintf("%s-%d", month.String(), day)
}

func checkError(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func checkErrMsg(err error, msg string) {
	if err != nil {
		log.WithFields(log.Fields{"msg": msg}).Panic(err)
	}
}

/*This file is for solely handling the database*/
func main() {
	quit := make(chan struct{})
	go introServer(config.REMOTE_YAML_CONFIG)
	db_channel := make(chan results.CCResults)
	go dbServer(db_channel)
	go dbWorker(db_channel, config.IP_LIST_LOCATION)
	go getGraphInfo(config.IP_LIST_LOCATION)
	<-quit
}
