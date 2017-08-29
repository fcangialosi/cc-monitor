#!/usr/bin/env Rscript

library(ggplot2)
args <- commandArgs(trailingOnly=TRUE)
thr_data_file <- args[1]
plot_filename <- args[2]
plot_title <- args[3]
png(plot_filename, width=1600, height=800)

thr_data <- read.csv(thr_data_file)

# add a line/point plot
thr_plot <- ggplot(thr_data, aes(x=time,y=throughput,colour=flow)) + geom_line()

theme_update(plot.title = element_text(hjust = 0.5, size=22))
theme_update(legend.key.size = unit(2,"cm"))

# add some axes
thr_plot <- thr_plot + theme(axis.title.y = element_text(size = rel(1.8)))
thr_plot <- thr_plot + theme(axis.title.x = element_text(size = rel(1.8)))

thr_plot <- thr_plot + theme(axis.text=element_text(size=14))
thr_plot <- thr_plot + theme(legend.text=element_text(size=14))

# add the title
thr_plot <- thr_plot + ggtitle(plot_title) + labs(x="Time(ms)",y="Throughput(mbps)") + 
ggsave(plot_filename)
dev.off()

