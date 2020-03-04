# ------------------------------------------------------------------------
from mcluseau/golang-builder:1.14.0 as build

# ------------------------------------------------------------------------
from debian:stretch
entrypoint ["/bin/dkl-local-server"]

env _uncache 1
run apt-get update \
 && apt-get install -y genisoimage gdisk dosfstools util-linux udev \
 && apt-get clean

run yes |apt-get install -y grub2 grub-pc-bin grub-efi-amd64-bin \
 && apt-get clean

run apt-get install -y ca-certificates curl openssh-client \
 && apt-get clean

run curl -L https://github.com/vmware/govmomi/releases/download/v0.21.0/govc_linux_amd64.gz | gunzip > /bin/govc && chmod +x /bin/govc

copy upload-vmware.sh govc.env /var/lib/direktil/

copy --from=build /go/bin/ /bin/
