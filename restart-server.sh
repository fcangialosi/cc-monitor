#!/bin/bash

sudo pkill cc-server
sudo ./cc-server > server.log 2>&1 &
