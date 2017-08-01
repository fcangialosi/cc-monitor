package main

import (
	"bufio"
	"io"
	"net"
	"os"
  "time"
  "strings"
  "fmt"
  "strconv"

	"../config"
	"../results"
	log "github.com/sirupsen/logrus"
)

func getIPList(ip_file string) (results.IPList, int) {
  ip_list := make(map[string](map[string][]string))
  var num_cycles int
	file, err := os.Open(ip_file)
	defer file.Close()
	checkError(err)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
    ip_line := scanner.Text() // line: IP UDP alg1 alg2 TCP alg1 alg2 ....
    ip_line_split := strings.Split(ip_line, " ")
    log.Info(ip_line_split)
    if len(ip_line_split) == 1 {
      num_cycles, err = strconv.Atoi(ip_line_split[0])
      if err != nil {
        log.WithFields(log.Fields{"err":err}).Error("error reading num_cycles")
      }
      continue
    }
    ip := ip_line_split[0]
    udp_algorithms := make([]string, 0)
    tcp_algorithms := make([]string, 0)
    udp := true
    for _, val := range ip_line_split[3:] { // first val is IP, 2nd val is location, 3rd val is "UDP"
      if val == "TCP" {
        udp = false
        continue
      }
      if udp {
        udp_algorithms = append(udp_algorithms, val)
      } else {
        tcp_algorithms = append(tcp_algorithms, val)
      }
    }

    alg_map := make(map[string][]string)
    alg_map["UDP"] = udp_algorithms
    alg_map["TCP"] = tcp_algorithms

    ip_list[ip] = alg_map
    log.WithFields(log.Fields{"IP": ip, "alg map": alg_map}).Info("IP LIST")
	}
	checkError(scanner.Err())
	return ip_list, num_cycles
}

func getIPLocation(ip_file string, input_ip string) string {
	file, err := os.Open(ip_file)
	defer file.Close()
	checkError(err)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
    ip_line := scanner.Text() // line: IP UDP alg1 alg2 TCP alg1 alg2 ....
    ip_line_split := strings.Split(ip_line, " ")
    ip := ip_line_split[0]
    location := ip_line_split[1]
    if ip == input_ip {
      return location
    }
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
			ip_list, num_cycles := getIPList(ip_file)
			// send it back to the server
			conn.Read(p)
			conn.Write(results.EncodeIPList(ip_list, num_cycles))
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
			defer conn.Close()
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
    go func(results.CCResults) {
      log.Info("got decoded report from channel")
      server_ip := report.ServerIP
      client_ip := strings.Split(report.ClientIP, ":")[0]
      current_date := currentDate()
      current_time := currentTime()

      // check if everything exists
      server_file := fmt.Sprintf("%s_logs", server_ip)
      location := getIPLocation(ip_file, server_ip)
      if ( location != "NOT_FOUND" ) {
        server_file = fmt.Sprintf("%s_logs", location)
      }
      filename := fmt.Sprintf("%s_%s.log", client_ip, current_time)
      path := fmt.Sprintf("exp_results/%s/%s", server_file, current_date)
      err := os.MkdirAll(path, 0777)
      if err != nil {
        log.WithFields(log.Fields{"err": err, "path": path}).Panic("Creating path to store results")
      }
      full_path := path + "/" + filename
      f, err := os.Create(full_path)
      checkErrMsg(err, "creating file for path " + full_path)
      log.WithFields(log.Fields{"path": full_path}).Info("Writing decoded results")
      defer f.Close()

     // marshall the struct
     b := results.EncodeCCResults(&report)
    checkErrMsg(err, "marshalling report into bytes")
    _, err = f.Write(b)
    checkErrMsg(err, "writing bytes to file")
    }(report)

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

  time.Sleep(time.Second*2)
  elapsed := time.Since(start)
  elapsed_ms := elapsed.Seconds()* float64(time.Second/time.Millisecond)
  log.WithFields(log.Fields{"elapsed": elapsed_ms}).Info("elapsed time")
  // get the ip list for tests
  ip_list, num_cycles := getIPList(config.IP_LIST_LOCATION)
  log.WithFields(log.Fields{"num_cycles":num_cycles}).Info(ip_list)

	quit := make(chan struct{})
	go introServer(config.IP_LIST_LOCATION)
	db_channel := make(chan results.CCResults)
	go dbServer(db_channel)
	go dbWorker(db_channel, config.IP_LIST_LOCATION)
	<-quit

}
