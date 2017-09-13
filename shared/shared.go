package shared

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

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
}

func ParseYAMLConfig(config_file string) (ServerList, int, time.Duration, bool) {
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
	exp_time, err := time.ParseDuration(config.Exp_time)
	if err != nil {
		log.Fatal("Config contains invalid exp_time, expected format: [0-9]?(s|m|h)")
	}
	return config.Servers, config.Num_cycles, exp_time, config.Lock_servers
}

func EncodeConfig(servers ServerList, num_cycles int, exp_time time.Duration, lock_servers bool) []byte {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(servers)
	e.Encode(num_cycles)
	e.Encode(exp_time)
	e.Encode(lock_servers)
	return w.Bytes()
}
