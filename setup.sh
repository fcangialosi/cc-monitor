#!/bin/bash

# Frank Cangialosi
# September, 2017
#
# This should always be run on a cc-monitor server before starting the server process
# Many of the settings are reset on reboot and need to be set up correctly each time
# 
# Tested on Ubuntu 16.04.2 LTS, 4.10.0+

if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root" 
   exit 1
fi

###############################################################################
# Helper functions
###############################################################################
# Returns true if command exists on this machine
cmd_exists() {
	type "$1" >/dev/null 2>&1 ;
}
###############################################################################

echo "===> Installing necessary dependencies"
sudo apt-get install makepp libboost-dev libprotobuf-dev protobuf-compiler libjemalloc-dev iperf

echo "===> Loading necessary kernel modules (tcpprobe, tcp_bbr, tcp_vegas)..."
# Load TCP BBR
sudo modprobe tcp_bbr
# Load TCP Vegas
sudo modprobe tcp_vegas

echo "===> Setting kernel params..."

# expand the tcp receive buffer size
sudo sysctl -w net.core.rmem_max=8388608
sudo sysctl -w net.core.rmem_default=8388608

# make bbr and vegas available to applications
ALGS=`cat /proc/sys/net/ipv4/tcp_available_congestion_control`
echo $ALGS > /proc/sys/net/ipv4/tcp_allowed_congestion_control

# enable fq, which is necessary for bbr pacing to work correctly
sudo sysctl -w net.core.default_qdisc=fq
sudo tc qdisc replace dev eth0 root fq
echo "===> Done."
echo "===> IMPORTANT: Before running BBR, check that the fq qdisc is currently turned on using 'sudo tc qdisc show'"


