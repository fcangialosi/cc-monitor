package shared

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"cc-monitor/config"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

func MachineHostname(server string) string {
	file, err := os.Open(config.HOSTNAME_LOCATION_FILE)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), " ")
		if line[1] == server {
			return line[0]
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return server
}
func ParseAlgParams(line string) (params map[string]string) {
	params = make(map[string]string)
	sp := strings.Split(line, " ")
	for _, param := range sp {
		kv := strings.Split(param, "=")
		if len(kv) == 2 {
			params[kv[0]] = kv[1]
		}
	}
	return
}

func FriendlyAlgString(line string) string {
	algName := strings.Split(line, " ")[0]
	if algName != "ccp-nimbus" {
		return algName
	}
	params := ParseAlgParams(line)
	useSwitching := false
	if val, ok := params["useSwitching"]; ok {
		if val == "true" {
			useSwitching = true
		}
	}
	flowMode := ""
	if val, ok := params["flowMode"]; ok {
		flowMode = val
	}

	if useSwitching {
		return "nimbus"
	} else if !(useSwitching) && flowMode == "DELAY" {
		return "nimbus_delay"
	} else if !(useSwitching) && flowMode == "XTCP" {
		return "nimbus_tcpcomp"
	} else {
		return "nimbus"
	}

}

func ParseAlg(line string) (string, string) {
	sp := strings.Split(line, "/")
	return sp[0], sp[1]
}

type ServerList []map[string][]string
type YAMLConfig struct {
	Num_cycles   int
	Exp_time     string
	Lock_servers bool
	Servers      ServerList
	Retry_locked bool
	Pick_servers int
}

func ReadYAMLConfig(config_file string) *YAMLConfig {
	config := YAMLConfig{}
	data, err := ioutil.ReadFile(config_file)
	if err != nil {
		log.Warn("Hint: make sure you're only using spaces, not tabs!")
		log.Fatal("Error reading config file: ", err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Warn("Hint: make sure you're only using spaces, not tabs!")
		log.Fatal("Error parsing config file: ", err)
	}
	return &config
}

func ParseYAMLConfig(config_file string) (ServerList, int, time.Duration, bool, bool, int) {
	config := ReadYAMLConfig(config_file)
	exp_time, err := time.ParseDuration(config.Exp_time)
	if err != nil {
		log.Fatal("Config contains invalid exp_time, expected format: [0-9]?(s|m|h)")
	}
	return config.Servers, config.Num_cycles, exp_time, config.Lock_servers, config.Retry_locked, config.Pick_servers
}

func EncodeConfig(config *YAMLConfig) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(config.Num_cycles)
	e.Encode(config.Exp_time)
	e.Encode(config.Lock_servers)
	e.Encode(config.Servers)
	e.Encode(config.Retry_locked)
	e.Encode(config.Pick_servers)
	return w.Bytes()
}

func DecodeConfig(data []byte) YAMLConfig {
	config := YAMLConfig{}
	r := bytes.NewBuffer(data)
	if data == nil || len(data) < 1 {
		log.Fatal("Error decoding config")
	}
	d := gob.NewDecoder(r)
	d.Decode(&config.Num_cycles)
	d.Decode(&config.Exp_time)
	d.Decode(&config.Lock_servers)
	d.Decode(&config.Servers)
	d.Decode(&config.Retry_locked)
	d.Decode(&config.Pick_servers)
	return config
}

func UTCTimeString() string {
	return fmt.Sprintf(time.Now().UTC().Format("20060102150405"))
}

func RemoveExpTime(alg string) string {
	algSp := strings.Split(alg, "_")
	for i, _ := range algSp {
		if algSp[i] == "exp" {
			algSp = append(algSp[:i], algSp[i+2:]...)
			break
		}
	}
	return strings.Join(algSp, "_")
}

func RemoveSpacesAlg(alg string) string {
	/*Replaces spaces with _ in algorithm name for graph filenames*/
	alg_broken := strings.Split(alg, " ")
	return strings.Join(alg_broken, "_")
}

func checkErr(err error, msg string) {
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Panic(msg)
	}
}

/*
When a report is sent from client, graphs and logfiles are created and stored in a location:
GRAPH_LOCATION/serverip-clientip/UTCTime/
Params: client IP, server IP, clientTime - in form specificed by UTCTimeString()
returns: path to the logfolder
*/
func CreateLogFolder(clientIP string, serverIP string, clientTime string) string {
	serverLocation := MachineHostname(serverIP) // friendly name
	clientLocation := MachineHostname(clientIP)

	path := fmt.Sprintf("%s%s-%s/%s", config.PATH_TO_GRAPH_RESULTS, serverLocation, clientLocation, clientTime) // format DIR/server-client/UTCTime
	log.Info("Making log at path: ", path)
	err := os.MkdirAll(path, 0777)
	checkErr(err, fmt.Sprintf("Creating log directory for server %s and client %s at time %s\n", serverIP, clientIP, clientTime))

	return path
}

/*
Opens a filename and writes bytes to said file.
*/
func WriteBytes(bytes []byte, filename string) string {
	f, err := os.Create(filename)
	checkErr(err, fmt.Sprintf("Creating filename with name %s\n", filename))
	defer f.Close()
	_, err = f.Write(bytes)
	checkErr(err, fmt.Sprintf("Writing bytes to file with filename %s\n", filename))
	return filename
}

/*
Naming scheme for CCP log given a client and server IP
*/
func CCPLogLocation(clientIP string, serverIP string) string {
	return fmt.Sprintf("%s%s-%s", config.DB_SERVER_CCP_TMP, MachineHostname(serverIP), MachineHostname(clientIP))
}

/*
Naming scheme for the graph that compares bytes sent across algorithms.
Returns (graphtitle, graphname)
*/
func CompareGraphName(clientIP string, serverIP string, clientTime string) (string, string) {
	serverLocation := MachineHostname(serverIP) // friendly name
	clientLocation := MachineHostname(clientIP)
	graphTitle := fmt.Sprintf("%s_to_%s", clientLocation, serverLocation)
	graphFilename := fmt.Sprintf("%s-%s-%s_data_transfer", clientIP, serverIP, clientTime)

	return graphTitle, graphFilename
}

/*
arguments for throughput graph script
graphFilename => return value from CompareGraphName (main graphing script run first)
*/
func ThroughputGraphName(graphFilename string, alg string) (string, string, string) {
	graphTitle := fmt.Sprintf("%s_Throughput_Delay", alg)
	throughputLogfile := fmt.Sprintf("%s_%s", alg, graphFilename)
	outfile := fmt.Sprintf("%s_throughput", alg)
	return graphTitle, throughputLogfile, outfile
}
