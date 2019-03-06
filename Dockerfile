# ------------------------------------------------------------------------
from golang:1.12.0 as build

env CGO_ENABLED 0
env pkg         novit.nc/direktil/local-server

copy vendor /go/src/${pkg}/vendor
copy pkg    /go/src/${pkg}/pkg
copy cmd    /go/src/${pkg}/cmd
workdir /go/src/${pkg}
run go test ./... \
 && go install ./cmd/...

# ------------------------------------------------------------------------
from debian:stretch
entrypoint ["/bin/dkl-local-server"]

run apt-get update \
 && apt-get install -y genisoimage gdisk dosfstools util-linux udev \
 && apt-get clean

run yes |apt-get install -y grub2 grub-pc-bin grub-efi-amd64-bin \
 && apt-get clean

copy --from=build /go/bin/ /bin/
