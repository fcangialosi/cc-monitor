#!/bin/bash

SITE_SERVER=nimbus2000.csail.mit.edu

git pull --rebase
make
scp ./cc-client $SITE_SERVER:~/dist-site/res/cc-client-darwin
NEW_VERSION=`grep "CLIENT_VERSION :=" client/main.go | tr '"' ' ' | awk '{print $3 "."}'`
echo $NEW_VERSION
ssh $SITE_SERVER "./update-site.sh $NEW_VERSION."
