#!/bin/bash

echo "> git pull"
sudo -u ubuntu git -C /home/ubuntu/ccp pull --rebase --force
CI=`sudo -u ubuntu git -C /home/ubuntu/ccp rev-parse HEAD`
echo -e "\nNow at commit ${CI::6}\n"

echo "> make"

GOPATH=/home/ubuntu/go-work sudo -E -u ubuntu make -C /home/ubuntu/ccp
