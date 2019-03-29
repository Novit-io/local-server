#! /bin/bash

dls_url="$1"

set -ex

mount -o remount,rw /boot

if [ -e /boot/previous ]; then
    rm -fr /boot/previous
fi

if [ -e /boot/current ]; then
    mv /boot/current /boot/previous
fi

curl $dls_url/me/boot.tar |tar xv -C /boot
sync

