package config

// conversion
const BYTES_TO_MBITS = .000008

// IP
const SERVER_IP = "127.0.0.1"

const DB_IP = "34.227.95.116"

// DB
const DB_NAME = "cc_test_data"
const DB_TABLE_NAME = "cc_results"
const DB_USERNAME = "user"
const DB_PASSWORD = "password"

// HEADER
const HEADER_SIZE = 32
const RECEIVE_TIMESTAMP_START = 24
const RECEIVE_TIMESTAMP_END = 32

// Ports
const PING_UDP_SERVER_PORT = "10100"
const PING_TCP_SERVER_PORT = "10101"
const MEASURE_SERVER_PORT = "10102"
const DB_SERVER_PORT = "10103"
const MAX_PORT = 65535

// Ping server constants
const PING_SIZE_BYTES = 1492

// Measure server constants
const LARGE_BUF_SIZE = 4096
const MAX_REQ_SIZE = 128
const TCP_CONGESTION = 0xd
const MEAN_ON_TIME_MS = 10000
const MEAN_OFF_TIME_MS = 3000
const NUM_CYCLES = 5
const TRANSFER_BUF_SIZE = 2048
const PATH_TO_GENERIC_CC = "/home/ubuntu/genericCC/sender"
const PATH_TO_REMY_CC = "/home/ubuntu/cc-monitor/bigbertha-100x.dna.5"
const INITIAL_X_VAL = 1000
const FIN = "FIN"
const ACK = "ACK"
const FIN_LEN = 3
const ACK_LEN = 3
const START_FLOW = "START_FLOW"
const START_FLOW_LEN = 10
const START = "START"
const END = "END"

// alg names
const REMY = "remy"
const CUBIC = "cubic"
const BBR = "bbr"
