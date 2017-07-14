package config

// conversion
const BYTES_TO_MBITS = 8000000

// IP
const SERVER_IP = "127.0.0.1"

// HEADER
const HEADER_SIZE = 28
const RECEIVE_TIMESTAMP_START = 20
const RECEIVE_TIMESTAMP_END = 28

// Ports
const PING_SERVER_PORT = "10100"
const MEASURE_SERVER_PORT = "10101"
const DB_SERVER_PORT = "10102"

// Ping server constants
const PING_SIZE_BYTES = 1492

// Measure server constants
const MAX_REQ_SIZE = 128
const TCP_CONGESTION = 0xd
const MEAN_ON_TIME_MS = 10000
const MEAN_OFF_TIME_MS = 3000
const NUM_CYCLES = 5
const SEND_BUF_BYTES = 1492
const PATH_TO_GENERIC_CC = "./sender"
const PATH_TO_REMY_CC = "./RemyCC-2014-100x.dna"

// DB server constants
