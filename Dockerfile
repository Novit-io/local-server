# ------------------------------------------------------------------------
from golang:1.11.5 as build

env pkg novit.nc/direktil/local-server
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

copy --from=build /go/bin/ /bin/
