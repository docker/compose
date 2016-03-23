#!/usr/bin/env bash
set -e

rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/Sirupsen/logrus master
clone git github.com/cloudfoundry/gosigar master
clone git github.com/codegangsta/cli master
clone git github.com/coreos/go-systemd master
clone git github.com/cyberdelia/go-metrics-graphite master
clone git github.com/docker/docker master
clone git github.com/docker/go-units master
clone git github.com/godbus/dbus master
clone git github.com/golang/glog master
clone git github.com/golang/protobuf master
clone git github.com/opencontainers/runc 5f182ce7380f41b8c60a2ecaec14996d7e9cfd4a
clone git github.com/opencontainers/specs/specs-go 3ce138b1934bf227a418e241ead496c383eaba1c
clone git github.com/rcrowley/go-metrics master
clone git github.com/satori/go.uuid master
clone git github.com/syndtr/gocapability master
clone git github.com/vishvananda/netlink master
clone git github.com/Azure/go-ansiterm master
clone git golang.org/x/net master https://github.com/golang/net.git
clone git google.golang.org/grpc master https://github.com/grpc/grpc-go.git
clone git github.com/seccomp/libseccomp-golang master

clean
