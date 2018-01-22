#!/bin/bash
outfile=$1 #outfile for this algorithm
pngname=$2
graphtitle=$3 #title for this algorithm
RESULTS=$4 # final location to move the file
ALGFOLDERRESULTS=$5
numFlows=$6
pngfile="$outfile.png"
csvfile="${outfile}.csv"
delaycsvfile="${outfile}_delay.csv"
instcsvfile="${outfile}_inst.csv"
bandwidthcsvfile="${outfile}_bandwidth.csv"


cd "/home/ubuntu/cc-monitor/monitor_plots"
mv "/home/ubuntu/cc-monitor/$outfile.csv" "/home/ubuntu/cc-monitor/monitor_plots/$outfile.csv"
mv "/home/ubuntu/cc-monitor/${delaycsvfile}" "/home/ubuntu/cc-monitor/monitor_plots/$delaycsvfile"
mv "/home/ubuntu/cc-monitor/${instcsvfile}" "/home/ubuntu/cc-monitor/monitor_plots/$instcsvfile"
mv "/home/ubuntu/cc-monitor/${bandwidthcsvfile}" "/home/ubuntu/cc-monitor/monitor_plots/$bandwidthcsvfile"
./graph_thr_del.r $csvfile $instcsvfile $delaycsvfile $pngfile $graphtitle
mv $pngfile "$RESULTS/$pngname.png"

# do the individual throughput delay files
for i in `seq 0 $numFlows`; do
    currfilename="${outfile}_${i}.png"
    currgraphtitle="${graphtitle}_trial-${i}"
    ./graph_individual_thr_del.r $csvfile $instcsvfile $delaycsvfile $currfilename $currgraphtitle $trial
    mv $currfilename "$ALGFOLDERRESULTS/$currfilename"
done

# remove these files - do not need them any more
mv "/home/ubuntu/cc-monitor/monitor_plots/$delaycsvfile" "$RESULTS/$delaycsvfile"
mv "/home/ubuntu/cc-monitor/monitor_plots/$bandwidthcsvfile" "$RESULTS/$bandwidthcsvfile"
rm "/home/ubuntu/cc-monitor/monitor_plots/$outfile.csv"
rm "/home/ubuntu/cc-monitor/monitor_plots/$instcsvfile"
echo "Wrote graph to $RESULTS/$pngname.png"
