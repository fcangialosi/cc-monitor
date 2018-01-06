#!/bin/bash

SITE_SERVER=nimbus

git pull
ssh $SITE_SERVER "rm /var/www/ccperf/ccperf-darwin"
scp ./ccperf $SITE_SERVER:/var/www/ccperf/ccperf-darwin
#NEW_VERSION=`grep "CLIENT_VERSION =" client/main.go | tr '"' ' ' | awk '{print $3 "."}'`
#echo $NEW_VERSION
#ssh $SITE_SERVER "./update-site.sh $NEW_VERSION"
