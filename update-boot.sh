#! /bin/bash

dls_url="$1"

set -ex

mount -o remount,rw /boot
rm -fr /boot/previous ||true
mv /boot/current /boot/previous
curl $dls_url/me/boot.tar |tar xv -C /boot
sync

