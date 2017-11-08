package config

const HOME = "/home/ubuntu/"
const STORAGE = "/home/ubuntu/exp_results/"

// conversion
const BYTES_TO_MBITS = .000008

const DB_IP = "34.226.119.144"

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
const PING_UDP_SERVER_PORT = "10100"
const PING_TCP_SERVER_PORT = "10101"
const MEASURE_SERVER_PORT = "10102"
const DB_SERVER_PORT = "10103"
const IP_SERVER_PORT = "10104"
const MAX_PORT = 65535
const OPEN_UDP_PORT = "10105"
const DB_GRAPH_PORT = "10105"
const CLIENT_UDP_PORT = "9876"
const SRTT_INFO_PORT = "10106"

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
const URL_PREFIX = DB_IP + ":8000"
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
