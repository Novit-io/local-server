# ------------------------------------------------------------------------
from golang:1.12.1-alpine as build
run apk add --update git

env CGO_ENABLED 0
arg GOPROXY

workdir /src
add go.sum go.mod ./
run go mod download

add . ./
run go test ./...
run go install ./cmd/...

# ------------------------------------------------------------------------
from debian:stretch
entrypoint ["/bin/dkl-local-server"]

run apt-get update \
 && apt-get install -y genisoimage gdisk dosfstools util-linux udev \
 && apt-get clean

run yes |apt-get install -y grub2 grub-pc-bin grub-efi-amd64-bin \
 && apt-get clean

copy --from=build /go/bin/ /bin/
