.PHONY: build test cover bench dump

export GOFLAGS := -ldflags=-checklinkname=0

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

dump:
	python3 scripts/dump-repo.py datura.txt