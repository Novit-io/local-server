# ------------------------------------------------------------------------
from golang:1.11.2 as build

env pkg novit.nc/direktil/local-server
run go get github.com/gobuffalo/packr/packr
copy vendor /go/src/${pkg}/vendor
copy cmd    /go/src/${pkg}/cmd
copy assets /go/src/${pkg}/cmd/dkl-local-server/assets
workdir /go/src/${pkg}
run packr -i /go/src/${pkg}/cmd/dkl-local-server \
 && go test ./... \
 && go install ./cmd/...

# ------------------------------------------------------------------------
from debian:stretch
entrypoint ["/bin/dkl-local-server"]

run apt-get update \
 && apt-get install -y genisoimage gdisk dosfstools util-linux udev \
 && apt-get clean

run yes |apt-get install -y grub2 grub-pc-bin grub-efi-amd64-bin \
 && apt-get clean

add scripts   /scripts
add assets    /assets
add efi-shim/ /shim
copy --from=build /go/bin/ /bin/
