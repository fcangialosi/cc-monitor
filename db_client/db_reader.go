package main
import (
	log "github.com/sirupsen/logrus"
  "../results"
  "os"
)

func checkErrMsg(err error, msg string) {
	if err != nil {
    log.WithFields(log.Fields{"msg": msg}).Panic(err)
	}
}

func readfile(filepath string) {

  if _, err := os.Stat(filepath); os.IsNotExist(err) {
    log.Panic("Must provide a valid filepath")
  }

  // open the file and read it
  f, err := os.Open(filepath)
  defer f.Close()
  checkErrMsg(err, "opening file")

  b := make([]byte, 50000000)
  // it is an encoded CCResults -> must decode it
  n, err := f.Read(b)
  checkErrMsg(err, "reading bytes into buf")
  results := results.DecodeCCResults(b[:n])
  log.Info(results)
}

func main(){
  argsWithoutProg := os.Args[1:]
  filepath := argsWithoutProg[0]
  log.Info(filepath)
  readfile(filepath)
}
