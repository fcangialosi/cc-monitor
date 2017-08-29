#!/usr/bin/env Rscript

library(ggplot2)
require(cowplot)
theme_set(theme_grey())
args <- commandArgs(trailingOnly=TRUE)
thr_data_file <- args[1]
del_data_file <- args[2]
plot_filename <- args[3]
plot_title <- args[4]
png(plot_filename, width=1600, height=800)

thr_data <- read.csv(thr_data_file)
del_data <- read.csv(del_data_file)
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
thr_plot <- thr_plot + ggtitle("Throughput") + labs(x="Time(s)",y="Throughput(mbps)")


# add a line/point plot
del_plot <- ggplot(del_data, aes(x=time,y=rtt,colour=flow)) + geom_line()


# add some axes
del_plot <- del_plot + theme(axis.title.y = element_text(size = rel(1.8)))
del_plot <- del_plot + theme(axis.title.x = element_text(size = rel(1.8)))

del_plot <- del_plot + theme(axis.text=element_text(size=14))
del_plot <- del_plot + theme(legend.text=element_text(size=14))

# add the title
del_plot <- del_plot + ggtitle("Delay") + labs(x="Time(s)",y="RTT(ms)")

plot.throughput <- thr_plot
plot.delay <- del_plot
title <- ggdraw() + draw_label(plot_title, fontface='bold', size=22)
p <- plot_grid(plot.throughput, plot.delay, labels = c('A', 'B'))
plot_grid(title, p, ncol=1, rel_heights=c(0.1, 1))


dev.off()

