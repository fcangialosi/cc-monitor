#!/bin/bash

SITE_SERVER=nimbus

git pull
scp ./cc-client $SITE_SERVER:/var/www/cctest/cc-client-darwin
#NEW_VERSION=`grep "CLIENT_VERSION =" client/main.go | tr '"' ' ' | awk '{print $3 "."}'`
#echo $NEW_VERSION
#ssh $SITE_SERVER "./update-site.sh $NEW_VERSION"
