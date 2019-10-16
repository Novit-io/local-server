# ------------------------------------------------------------------------
from mcluseau/golang-builder:1.13.1 as build

# ------------------------------------------------------------------------
from debian:stretch
entrypoint ["/bin/dkl-local-server"]

run apt-get update \
 && apt-get install -y genisoimage gdisk dosfstools util-linux udev \
 && apt-get clean

run yes |apt-get install -y grub2 grub-pc-bin grub-efi-amd64-bin \
 && apt-get clean

run apt-get install -y ca-certificates \
 && apt-get clean

copy --from=build /go/bin/ /bin/
