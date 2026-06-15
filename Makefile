.PHONY: build test cover bench

build:
	capnp compile -I ../../capnproto/go-capnp/std -ogo artifact.capnp dmt/server/server.capnp

test:
	capnp compile -I ../../capnproto/go-capnp/std -ogo artifact.capnp dmt/server/server.capnp
	go test -v ./...

cover:
	capnp compile -I ../../capnproto/go-capnp/std -ogo artifact.capnp dmt/server/server.capnp
	go test -cover -bench=. -benchmem ./...

bench:
	capnp compile -I ../../capnproto/go-capnp/std -ogo artifact.capnp dmt/server/server.capnp
	go test -run=^$$ -bench=. -benchmem ./...