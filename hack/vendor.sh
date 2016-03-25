#!/usr/bin/env bash
set -e

rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/Sirupsen/logrus 4b6ea7319e214d98c938f12692336f7ca9348d6b
clone git github.com/cloudfoundry/gosigar 3ed7c74352dae6dc00bdc8c74045375352e3ec05
clone git github.com/codegangsta/cli 9fec0fad02befc9209347cc6d620e68e1b45f74d
clone git github.com/coreos/go-systemd 7b2428fec40033549c68f54e26e89e7ca9a9ce31
clone git github.com/cyberdelia/go-metrics-graphite 7e54b5c2aa6eaff4286c44129c3def899dff528c
clone git github.com/docker/docker 9ff767bcc06c924fd669d881a34847aa4fbaab5e
clone git github.com/docker/go-units 5d2041e26a699eaca682e2ea41c8f891e1060444
clone git github.com/godbus/dbus e2cf28118e66a6a63db46cf6088a35d2054d3bb0
clone git github.com/golang/glog 23def4e6c14b4da8ac2ed8007337bc5eb5007998
clone git github.com/golang/protobuf 8d92cf5fc15a4382f8964b08e1f42a75c0591aa3
clone git github.com/opencontainers/runc 5f182ce7380f41b8c60a2ecaec14996d7e9cfd4a
clone git github.com/opencontainers/specs 3ce138b1934bf227a418e241ead496c383eaba1c
clone git github.com/rcrowley/go-metrics eeba7bd0dd01ace6e690fa833b3f22aaec29af43
clone git github.com/satori/go.uuid f9ab0dce87d815821e221626b772e3475a0d2749
clone git github.com/syndtr/gocapability 2c00daeb6c3b45114c80ac44119e7b8801fdd852
clone git github.com/vishvananda/netlink adb0f53af689dd38f1443eba79489feaacf0b22e
clone git github.com/Azure/go-ansiterm 70b2c90b260171e829f1ebd7c17f600c11858dbe
clone git golang.org/x/net 991d3e32f76f19ee6d9caadb3a22eae8d23315f7 https://github.com/golang/net.git
clone git google.golang.org/grpc a22b6611561e9f0a3e0919690dd2caf48f14c517 https://github.com/grpc/grpc-go.git
clone git github.com/seccomp/libseccomp-golang 1b506fc7c24eec5a3693cdcbed40d9c226cfc6a1

clean
