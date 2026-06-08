.PHONY: build

build:
	capnp compile -I ../../capnproto/go-capnp/std -ogo artifact.capnp

test:
	capnp compile -I ../../capnproto/go-capnp/std -ogo artifact.capnp
	go test -v ./...