#! /bin/sh

if [ $# -ne 2 ]; then
    echo "USAGE: $0 <device> <base url>"
fi

dev=$1
base_url=$2

: ${MP:=/mnt}

set -ex

mkdir -p $MP

[[ $dev =~ nvme ]] &&
    devp=${dev}p ||
    devp=${dev}

if vgdisplay storage; then
    # the system is already installed, just upgrade
    mount -t vfat ${devp}1 $MP
    curl ${base_url}/boot.tar |tar xv -C $MP
    umount $MP

else
    sgdisk --clear $dev

    curl ${base_url}/boot.img.lz4 |lz4cat >$dev

    sgdisk --move-second-header --new=3:0:0 $dev

    pvcreate ${devp}3
    vgcreate storage ${devp}3
fi

while umount $MP; do true; done
