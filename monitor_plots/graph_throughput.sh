#!/bin/bash

outfile=$1 #outfile for this algorithm
pngname=$2
graphtitle=$3 #title for this algorithm
pngfile="$outfile.png"
csvfile="${outfile}.csv"
delaycsvfile="${outfile}_delay.csv"
cd "/home/ubuntu/cc-monitor/monitor_plots"
RESULTS=$4 # final location to move the file
mv "/home/ubuntu/cc-monitor/$outfile.csv" "/home/ubuntu/cc-monitor/monitor_plots/$outfile.csv"
mv "/home/ubuntu/cc-monitor/${delaycsvfile}" "/home/ubuntu/cc-monitor/monitor_plots/$delaycsvfile"
./graph_thr_del.r $csvfile $delaycsvfile $pngfile $graphtitle
mv $pngfile "$RESULTS/$pngname.png"
rm "/home/ubuntu/cc-monitor/monitor_plots/$delaycsvfile"
rm "/home/ubuntu/cc-monitor/monitor_plots/$outfile.csv"
echo "Wrote graph to $RESULTS/$pngname.png"
