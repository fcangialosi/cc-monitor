#!/bin/bash

SITE_SERVER=frank@nimbus2000.csail.mit.edu

git pull
scp -P 222 ./cc-client $SITE_SERVER:~/dist-site/res/cc-client-darwin
NEW_VERSION=`grep "CLIENT_VERSION :=" client/main.go | tr '"' ' ' | awk '{print $3}'`
echo $NEW_VERSION
ssh -P 222 $SITE_SERVER "./update-site.sh $NEW_VERSION."
