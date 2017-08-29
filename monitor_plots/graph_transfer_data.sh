#!/bin/bash

logfile=$1
outfile=$2
graphtitle=$3
pngfile="$outfile.png"
RESULTS=$4 # final location to move the file
cd "/home/ubuntu/cc-monitor"
./reader $logfile $outfile
cd "/home/ubuntu/cc-monitor/monitor_plots"
mv "/home/ubuntu/cc-monitor/$outfile.csv" "$outfile.csv"
./cc_monitor.r "$outfile.csv" $pngfile $graphtitle
mv $pngfile "$RESULTS/file_transfer.png"
rm "$outfile.csv"
echo "Wrote graph to $RESULTS/file_transfer.png"
