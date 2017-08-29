#!/usr/bin/env Rscript

library(ggplot2)
require(cowplot)
theme_set(theme_grey())
args <- commandArgs(trailingOnly=TRUE)
thr_data_file <- args[1]
inst_thr_data_file <- args[2]
del_data_file <- args[3]
plot_filename <- args[4]
plot_title <- args[5]
png(plot_filename, width=1600, height=800)

thr_data <- read.csv(thr_data_file)
del_data <- read.csv(del_data_file)
inst_thr_data <- read.csv(inst_thr_data_file)
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
thr_plot <- thr_plot + ggtitle("Avg Throughput") + labs(x="Time(s)",y="Throughput(mbps)")



######## inst throughput ######
inst_thr_plot <- ggplot(inst_thr_data, aes(x=time,y=throughput,colour=flow)) + geom_line()


# add some axes
inst_thr_plot <- inst_thr_plot + theme(axis.title.y = element_text(size = rel(1.8)))
inst_thr_plot <- inst_thr_plot + theme(axis.title.x = element_text(size = rel(1.8)))

inst_thr_plot <- inst_thr_plot + theme(axis.text=element_text(size=14))
inst_thr_plot <- inst_thr_plot + theme(legend.text=element_text(size=14))

# add the title
inst_thr_plot <- inst_thr_plot + ggtitle("Inst Throughput") + labs(x="Time(s)",y="Throughput(mbps)")


##### del plot ######
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
plot.inst_throughput <- inst_thr_plot
title <- ggdraw() + draw_label(plot_title, fontface='bold', size=22)
first_col <- plot_grid(plot.throughput, plot.inst_throughput, ncol=1, labels=c('', ''))
p <- plot_grid(first_col, plot.delay, nrow=1, labels = c('Thr', 'Del'))
plot_grid(title, p, ncol=1, rel_heights=c(0.1, 1))


dev.off()

