#!/bin/bash

logfile=$1
outfile=$2
aggoutfile="${outfile}_agg.csv"
ellipseoutfile="${outfile}_ellipse.csv"
echo "Agg outfile: $aggoutfile"
graphtitle=$3
pngfile="$outfile.png"
aggpngfile="${outfile}_agg.png"
RESULTS=$4 # final location to move the file
cd "/home/ubuntu/cc-monitor"
echo "outfile arg passed to reader: $outfile"
./reader $logfile $outfile
# move the relevant datafiles here for GGplot
cd "/home/ubuntu/cc-monitor/monitor_plots"
mv "/home/ubuntu/cc-monitor/$outfile.csv" "$outfile.csv"
mv "/home/ubuntu/cc-monitor/$aggoutfile" "$aggoutfile"
mv "/home/ubuntu/cc-monitor/$ellipseoutfile" "$ellipseoutfile"

# run the graphing script
./cc_monitor.r "$outfile.csv" $pngfile $graphtitle
./agg.r "$aggoutfile" $aggpngfile $graphtitle


mv $pngfile "$RESULTS/file_transfer.png"
mv $aggpngfile "$RESULTS/agg_transfer.png"


rm "$outfile.csv"
# move the files back
mv "$aggoutfile" "$RESULTS/aggregate_points.csv"
mv "$ellipseoutfile" "$RESULTS/ellipse_points.csv"
echo "Wrote graph to $RESULTS/file_transfer.png"
