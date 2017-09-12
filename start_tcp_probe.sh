#!/bin/bash
mkdir -p /home/ubuntu/cc-monitor/probes/
cat /proc/net/tcpprobe | grep $1 > /home/ubuntu/cc-monitor/probes/$2
