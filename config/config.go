package config

import "strconv"

const USER = "ubuntu"
const HOME = "/home/ubuntu/"
const STORAGE = "/home/ubuntu/mnt/"
const GOPATH = "/home/ubuntu/go-work"

// conversion
const BYTES_TO_MBITS = .000008

const DB_IP = "54.162.11.158"

// DB
const DB_NAME = "cc_test_data"
const DB_TABLE_NAME = "cc_results"
const DB_USERNAME = "user"
const DB_PASSWORD = "password"
const IP_LIST_LOCATION = "/home/ubuntu/ip.txt"
const REMOTE_YAML_CONFIG = STORAGE + "remote-config.yaml"

// HEADER
const HEADER_SIZE = 32
const RECEIVE_TIMESTAMP_START = 24
const RECEIVE_TIMESTAMP_END = 32
const SEQNUM_START = 0
const SEQNUM_END = 4

// Ports
const DEFAULT_PORT = 10100

func PING_UDP_SERVER_PORT(base_port int) string { return strconv.Itoa(base_port + 0) }
func PING_TCP_SERVER_PORT(base_port int) string { return strconv.Itoa(base_port + 1) }
func MEASURE_SERVER_PORT(base_port int) string  { return strconv.Itoa(base_port + 2) }
func DB_SERVER_PORT() string                    { return strconv.Itoa(DEFAULT_PORT + 3) }
func IP_SERVER_PORT() string                    { return strconv.Itoa(DEFAULT_PORT + 4) }
func DB_GRAPH_PORT() string                     { return strconv.Itoa(DEFAULT_PORT + 5) }
func SRTT_INFO_PORT(base_port int) string       { return strconv.Itoa(base_port + 6) }
func CCP_UPDATE_PORT(base_port int) string      { return strconv.Itoa(base_port + 7) }
func SERVER_UPDATE_PORT(base_port int) string   { return strconv.Itoa(base_port + 8) }
func CLIENT_UDP_PORT(base_port int) string      { return strconv.Itoa(base_port + 9) }
func OPEN_UDP_PORT(base_port int) string        { return strconv.Itoa(base_port + 10) }

const MAX_PORT = 65535

// Ping server constants
const PING_SIZE_BYTES = 1280
const PING_INTERSEND_MS = 500

// Measure server constants
const LARGE_BUF_SIZE = 4096
const TCP_BUF_SIZE = 4096
const TCP_TRANSFER_SIZE = 20000
const MAX_REQ_SIZE = 256
const TCP_CONGESTION = 0xd
const MEAN_ON_TIME_MS = 10000
const MEAN_OFF_TIME_MS = 3000
const MIN_ON_TIME = 1000 // atleast send for one second
const MIN_OFF_TIME = 500
const NUM_CYCLES = 3
const TRANSFER_BUF_SIZE = 2048
const URL_PREFIX = "ccperf.csail.mit.edu/data" //DB_IP + ":8000"
const PATH_TO_GRAPH_SCRIPT = "/home/ubuntu/cc-monitor/monitor_plots/graph_transfer_data.sh"
const PATH_TO_GRAPH_THROUGHPUT_SCRIPT = "/home/ubuntu/cc-monitor/monitor_plots/graph_throughput.sh"
const PATH_TO_GRAPH_RESULTS = STORAGE
const PATH_TO_GENERIC_CC = "/home/ubuntu/genericCC/sender"
const PATH_TO_REMY_CC = "/home/ubuntu/cc-monitor/rats/140-160.dna.5"
const PATH_TO_RATS = "/home/ubuntu/cc-monitor/rats/"
const INITIAL_X_VAL = 1000
const FIN = "FIN"
const ACK = "ACK"
const FIN_LEN = 3
const ACK_LEN = 3
const START_FLOW = "START_FLOW"
const SERVER_LOCKED = "SERVER_LOCKED"
const VERSION_MISMATCH = "VERSION_MISMATCH"
const OTHER_ERROR = "OTHER_ERROR"
const START = "START"
const END = "END"
const TRAIN_LENGTH = 8

const PATH_TO_CCP = HOME + "ccp/ccpl"
const PATH_TO_PRIV_KEY = HOME + ".ssh/rach.pem"

const DB_SERVER = "ubuntu@" + DB_IP
const DB_SERVER_CCP_TMP = PATH_TO_GRAPH_RESULTS + "ccp_tmp/"

// alg names
const REMY = "remy"
const CUBIC = "cubic"
const BBR = "bbr"
const VEGAS = "vegas"
const RENO = "reno"

// protocol names
const TCP = "tcp"
const UDP = "udp"

// timeout for reading
const CLIENT_TIMEOUT = 10 // 10 seconds before the client times out
const TCP_TIMEOUT = 15    // 15 second to try to get data
const MINUTE_TIMEOUT = 60
const HALF_MINUTE_TIMEOUT = 30
const CONNECT_TIMEOUT = 10
const MAX_CONNECT_ATTEMPTS = 6

//const LOCKED_RETRIES = 10
const RETRY_WAIT = 60

// files
const LOCAL_PROGRESS_FILE = "/tmp/cc-client_progress.log"
const LOCAL_RESULTS_FILE = "/tmp/cc-client_results.log"
const HOSTNAME_LOCATION_FILE = "/home/ubuntu/cc-monitor/locations.txt"
