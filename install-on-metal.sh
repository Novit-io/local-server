#! /bin/sh

if [ $# -ne 2 ]; then
    echo "USAGE: $0 <device> <tar url>"
fi

dev=$1
tar_url=$2

: ${MP:=/mnt}

set -ex

[[ $dev =~ nvme ]] &&
    devp=${dev}p ||
    devp=${dev}

vgdisplay storage || {
    sgdisk --clear $dev
    sgdisk \
        --new=0:4096:+2G --typecode=0:EF00 -c 0:boot \
        --new=0:0:+2M    --typecode=0:EF02 -c 0:BIOS-BOOT  \
        --new=0:0:0      --typecode=0:FFFF -c 0:data \
        --hybrid=1:2 \
        --print $dev

    mkfs.vfat -n DKLBOOT ${devp}1

    pvcreate ${devp}3
    vgcreate storage ${devp}3
}

while umount $MP; do true; done

mount -t vfat ${devp}1 $MP
curl $tar_url |tar xv -C $MP
umount $MP
