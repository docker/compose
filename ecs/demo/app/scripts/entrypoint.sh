#! /bin/sh

if [ "${LOCALDOMAIN}" != ""  ]; then echo "search ${LOCALDOMAIN}" >> /etc/resolv.conf; fi
exec "$@" 
