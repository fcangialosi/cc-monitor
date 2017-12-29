#!/bin/bash

echo "> git pull"
sudo -u ubuntu git -C /home/ubuntu/cc-monitor pull --rebase --force
CI=`sudo -u ubuntu git -C /home/ubuntu/cc-monitor rev-parse HEAD`
echo -e "\nNow at commit ${CI::6}\n"

echo "> make"

GOPATH=/home/ubuntu/go-work sudo -E -u ubuntu make -C /home/ubuntu/cc-monitor
