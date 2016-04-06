# Telemetry

Currently containerd only outputs metrics to stdout but will support dumping to various backends in the future.

```
[containerd] 2015/12/16 11:48:28 timer container-start-time
[containerd] 2015/12/16 11:48:28   count:              22
[containerd] 2015/12/16 11:48:28   min:          25425883
[containerd] 2015/12/16 11:48:28   max:         113077691
[containerd] 2015/12/16 11:48:28   mean:         68386923.27
[containerd] 2015/12/16 11:48:28   stddev:       20928453.26
[containerd] 2015/12/16 11:48:28   median:       65489003.50
[containerd] 2015/12/16 11:48:28   75%:          82393210.50
[containerd] 2015/12/16 11:48:28   95%:         112267814.75
[containerd] 2015/12/16 11:48:28   99%:         113077691.00
[containerd] 2015/12/16 11:48:28   99.9%:       113077691.00
[containerd] 2015/12/16 11:48:28   1-min rate:          0.00
[containerd] 2015/12/16 11:48:28   5-min rate:          0.01
[containerd] 2015/12/16 11:48:28   15-min rate:         0.01
[containerd] 2015/12/16 11:48:28   mean rate:           0.03
[containerd] 2015/12/16 11:48:28 counter containers
[containerd] 2015/12/16 11:48:28   count:               1
[containerd] 2015/12/16 11:48:28 counter events
[containerd] 2015/12/16 11:48:28   count:              87
[containerd] 2015/12/16 11:48:28 counter events-subscribers
[containerd] 2015/12/16 11:48:28   count:               2
[containerd] 2015/12/16 11:48:28 gauge goroutines
[containerd] 2015/12/16 11:48:28   value:              38
[containerd] 2015/12/16 11:48:28 gauge fds
[containerd] 2015/12/16 11:48:28   value:              18
```
