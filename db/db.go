package main

import (
	"database/sql"
	"io"
	"net"

	"../config"
	"../results"
	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

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
		if err != nil {
			log.Warning(err)
		}
		go func(c *net.TCPConn) {
			defer conn.Close()
			p := make([]byte, config.TRANSFER_BUF_SIZE)
			report_bytes := make([]byte, 0)
			ack := []byte(config.ACK)
			// read until client sends "end"
			for {
				n, err := conn.Read(p)
				if err == io.EOF {
					break // client closes the TCP connection
				}
				conn.Write(ack)
				checkError(err)
				if string(p[:n]) == config.FIN {
					log.Info("ending reading data because received fin")
					break
				}
				report_bytes = append(report_bytes, p[:n]...)
				log.WithFields(log.Fields{"n": n}).Info("bytes received")
			}
			// send this job to the dbWorker to upload to the database
			ch <- results.DecodeCCResults(report_bytes)
		}(conn)
	}

}

func dbWorker(ch chan results.CCResults) {
	// TODO reads jobs from channel, writes them to db
	// sets up connection with the database
  connection_info := config.DB_USERNAME + ":" + config.DB_PASSWORD + "@tcp(localhost:3306)" + "/" + config.DB_NAME + "?charset=utf8"
	//connection_info := "goserver:password@tcp(" + config.DB_IP + ":3306)/" + config.DB_NAME + "?charset=utf8"
	db, err := sql.Open("mysql", connection_info)
	defer db.Close()
	checkError(err)

	// check connection to db
	err = db.Ping()
	checkError(err)

	// read jobs from resuls channel forever
	for {
		report := <-ch
		db_result := results.BreakUpCCResult(&report)
		// TODO: add in bbr for realz
		remy_blob := db_result[config.REMY]
		cubic_blob := db_result[config.CUBIC]
		bbr_blob := []byte("bbr")
		stmt, err := db.Prepare("INSERT " + config.DB_TABLE_NAME + " SET remy=?,cubic=?,bbr=?,timestamp=NOW()")
		checkError(err)
		_, err = stmt.Exec(remy_blob, cubic_blob, bbr_blob)
		checkError(err)
	}
}

func checkError(err error) {
	if err != nil {
		log.Panic(err)
	}
}

/*This file is for solely handling the database*/
func main() {
	quit := make(chan struct{})
  log.Info("main is happening")
	db_channel := make(chan results.CCResults)
	go dbServer(db_channel)
	go dbWorker(db_channel)
  <-quit

}
