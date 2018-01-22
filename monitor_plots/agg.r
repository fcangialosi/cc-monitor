#!/usr/bin/env Rscript

library(ggplot2)
args <- commandArgs(trailingOnly=TRUE)
data_file <- args[1]
plot_title <- args[2]
title <- args[3]
png(plot_title, width=1600, height=800)

# read the data
data <- read.csv(data_file)
myplot <- ggplot(data,aes(x=delay,y=throughput,colour=algorithm)) +
      geom_point(size=8) + stat_ellipse(level=.3935) + scale_x_reverse()

# format the text in the plot
theme_update(plot.title = element_text(hjust = 0.5, size=22))
theme_update(legend.key.size = unit(2,"cm"))

# add some axes
myplot <- myplot + theme(axis.title.y = element_text(size = rel(1.8)))
myplot <- myplot + theme(axis.title.x = element_text(size = rel(1.8)))

myplot <- myplot + theme(axis.text=element_text(size=14))
myplot <- myplot + theme(legend.text=element_text(size=14))

# add the title
filepath <- paste('~/cc-monitor/monitor_plots/',plot_title)
myplot <- myplot + ggtitle(title) + labs(x="Mean delay(ms)",y="Throughput(mbps)") + 
ggsave(plot_title)
dev.off()
