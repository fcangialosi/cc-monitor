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

	"../config"
	"../results"
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

	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.IP_SERVER_PORT)
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
			p := make([]byte, config.LARGE_BUF_SIZE)
			defer conn.Close()
			ip_list, ip_order, num_cycles := results.GetIPList(ip_file)
			// send it back to the server
			conn.Read(p)
			conn.Write(results.EncodeIPList(ip_list, ip_order, num_cycles))
		}(conn)
	}

}
func dbServer(ch chan results.CCResults) {
	log.Info("In db server function")
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.DB_SERVER_PORT)
	if err != nil {
		log.Fatal(err)
	}

	server, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := server.AcceptTCP()
		log.Info("Got a report to read from the client")
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
				//log.WithFields(log.Fields{"n": n}).Info("bytes received")
			}
			log.Info("Finished reading the report")
			report := results.DecodeCCResults(report_bytes)
			report.ClientIP = conn.RemoteAddr().String() // client didn't put in right IP for some reason
			ch <- report
		}(conn)
	}

}

func dbWorker(ch chan results.CCResults, ip_file string) {
	// TODO reads jobs from channel, writes them to db
	// db in this case is just writing a file with the results

	// read jobs from results channel forever
	for {
		report := <-ch
		// spawn go routine to deal with this report
		go func(rep results.CCResults) {
			log.Info("got decoded report from channel")
			server_ip := rep.ServerIP
			client_ip := strings.Split(rep.ClientIP, ":")[0]
			current_date := currentDate()
			current_time := rep.SendTime // client will later ask for where the graph is

			// check if everything exists
			server_file := fmt.Sprintf("%s_logs", server_ip)
			location := getIPLocation(ip_file, server_ip)
			if location != "NOT_FOUND" {
				server_file = fmt.Sprintf("%s_logs", location)
			}
			filename := fmt.Sprintf("%s_%s.log", client_ip, current_time)
			path := fmt.Sprintf("/home/ubuntu/exp_results/%s/%s", server_file, current_date)
			err := os.MkdirAll(path, 0777)
			if err != nil {
				log.WithFields(log.Fields{"err": err, "path": path}).Panic("Creating path to store results")
			}

			shellCommand(fmt.Sprintf("mv %s/%s-%s/* %s", config.DB_SERVER_CCP_TMP, server_ip, client_ip, path), true)

			full_path := path + "/" + filename
			f, err := os.Create(full_path)
			checkErrMsg(err, "creating file for path "+full_path)
			log.WithFields(log.Fields{"path": full_path}).Info("Writing decoded results")
			defer f.Close()

			// marshall the struct
			b := results.EncodeCCResults(&rep)
			checkErrMsg(err, "marshalling report into bytes")
			_, err = f.Write(b)
			checkErrMsg(err, "writing bytes to file")

			// make the graph -> name it according to the time and location
			graph_title := fmt.Sprintf("Transfer_to_%s_AWS", location)
			graph_location := fmt.Sprintf("%s_%s", current_time, location)
			graph_directory := fmt.Sprintf("%s/%s/%s/%s", config.PATH_TO_GRAPH_RESULTS, server_file, current_date, current_time)
			err = os.MkdirAll(graph_directory, 0777)
			if err != nil {
				log.WithFields(log.Fields{"err": err, "path": path}).Panic("Creating graph path to store results")
			}

			args := []string{full_path, graph_location, graph_title, graph_directory}
			cmd := exec.Command(config.PATH_TO_GRAPH_SCRIPT, args...) // graphing scripts  moves the image to file with the python web server running
			cmd.Stdout = os.Stdout
			if err = cmd.Run(); err != nil {
				log.Info("Error in running graphing script")
				log.Error(err)
			}

			// now make the throughput graph for each of the algorithms
			for alg, _ := range rep.Throughput {
				log.Info("Alg is ", alg)
				// log for graphing script
				title := fmt.Sprintf("%s_Throughput", alg)
				thr_log := fmt.Sprintf("%s_%s", alg, graph_location)
				outfile := fmt.Sprintf("%s_throughput", alg)
				graph_args := []string{thr_log, outfile, title, graph_directory}
				log.Info(graph_args)

				cmd = exec.Command(config.PATH_TO_GRAPH_THROUGHPUT_SCRIPT, graph_args...)
				cmd.Stdout = os.Stdout
				if err = cmd.Run(); err != nil {
					log.Info("Error in running graphing script for throughput graphs")
					log.Error(err)
				}
			}
		}(report)

	}
}

func getGraphInfo(ip_file string) {
	log.Info("In get graph info server function")
	laddr, err := net.ResolveTCPAddr("tcp", ":"+config.DB_GRAPH_PORT)
	if err != nil {
		log.Fatal(err)
	}

	server, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := server.AcceptTCP()
		log.Info("Got a report to read from the client")
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
			server_file := fmt.Sprintf("%s", server_ip)
			current_time := report.SendTime
			current_date := currentDate()

			location := getIPLocation(ip_file, server_ip)
			if location != "NOT_FOUND" {
				server_file = fmt.Sprintf("%s", location)
			}
			path := fmt.Sprintf("%s/%s/%s", server_file, current_date, current_time)
			// find the correct URL and return
			URL := config.URL_PREFIX + "/" + path
			conn.Write([]byte(URL))
		}(conn)
	}
}

func currentDate() string {
	_, month, day := time.Now().Date()
	return fmt.Sprintf("%s-%d", month.String(), day)
}

func currentTime() string {
	hour, min, sec := time.Now().Clock()
	return fmt.Sprintf("%d.%d.%d", hour, min, sec)
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
	log.Info(currentDate())
	log.Info(currentTime())
	start := time.Now()

	time.Sleep(time.Second * 2)
	elapsed := time.Since(start)
	elapsed_ms := elapsed.Seconds() * float64(time.Second/time.Millisecond)
	log.WithFields(log.Fields{"elapsed": elapsed_ms}).Info("elapsed time")
	// get the ip list for tests
	ip_list, ip_order, num_cycles := results.GetIPList(config.IP_LIST_LOCATION)
	log.WithFields(log.Fields{"num_cycles": num_cycles, "order": ip_order}).Info(ip_list)

	quit := make(chan struct{})
	go introServer(config.IP_LIST_LOCATION)
	db_channel := make(chan results.CCResults)
	go dbServer(db_channel)
	go dbWorker(db_channel, config.IP_LIST_LOCATION)
	go getGraphInfo(config.IP_LIST_LOCATION)
	<-quit

}
