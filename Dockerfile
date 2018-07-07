from golang:1.10.3 as build
env pkg novit.nc/direktil/local-server
add . /go/src/${pkg}
workdir /go/src/${pkg}
run go vet ./... \
 && go test ./... \
 && go install .

from debian:stable
entrypoint ["/bin/local-server"]

run apt-get update \
 && apt-get install -y genisoimage grub grub-pc-bin \
 && apt-get clean

copy --from=build /go/bin/local-server /bin/
