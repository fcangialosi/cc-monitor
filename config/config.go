package config

// conversion
const BYTES_TO_MBITS = .000008

// IP
const SERVER_IP = "127.0.0.1"

// HEADER
const HEADER_SIZE = 32
const RECEIVE_TIMESTAMP_START = 24
const RECEIVE_TIMESTAMP_END = 32

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
const TRANSFER_BUF_SIZE = 2048
const PATH_TO_GENERIC_CC = "/home/ubuntu/genericCC/sender"
const PATH_TO_REMY_CC = "/home/ubuntu/cc-monitor/bigbertha-100x.dna.5"
