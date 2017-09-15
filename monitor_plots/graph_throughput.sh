#!/bin/bash
outfile=$1 #outfile for this algorithm
pngname=$2
graphtitle=$3 #title for this algorithm
pngfile="$outfile.png"
csvfile="${outfile}.csv"
delaycsvfile="${outfile}_delay.csv"
instcsvfile="${outfile}_inst.csv"
bandwidthcsvfile="${outfile}_bandwidth.csv"
cd "/home/ubuntu/cc-monitor/monitor_plots"
RESULTS=$4 # final location to move the file
mv "/home/ubuntu/cc-monitor/$outfile.csv" "/home/ubuntu/cc-monitor/monitor_plots/$outfile.csv"
mv "/home/ubuntu/cc-monitor/${delaycsvfile}" "/home/ubuntu/cc-monitor/monitor_plots/$delaycsvfile"
mv "/home/ubuntu/cc-monitor/${instcsvfile}" "/home/ubuntu/cc-monitor/monitor_plots/$instcsvfile"
mv "/home/ubuntu/cc-monitor/${bandwidthcsvfile}" "/home/ubuntu/cc-monitor/monitor_plots/$bandwidthcsvfile"
./graph_thr_del.r $csvfile $instcsvfile $delaycsvfile $pngfile $graphtitle
mv $pngfile "$RESULTS/$pngname.png"
mv "/home/ubuntu/cc-monitor/monitor_plots/$delaycsvfile" "$RESULTS/$delaycsvfile"
mv "/home/ubuntu/cc-monitor/monitor_plots/$bandwidthcsvfile" "$RESULTS/$bandwidthcsvfile"
rm "/home/ubuntu/cc-monitor/monitor_plots/$outfile.csv"
rm "/home/ubuntu/cc-monitor/monitor_plots/$instcsvfile"
echo "Wrote graph to $RESULTS/$pngname.png"
