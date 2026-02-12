.PHONY: build vet test clean setup

NWEP_VENDOR := vendor/github.com/usenwep/nwep-go
STAMP := $(NWEP_VENDOR)/.nwep-setup

build: setup
	go build -mod=vendor ./...

vet: setup
	go vet -mod=vendor ./...

test: setup
	go test -mod=vendor ./...

setup: $(STAMP)

$(STAMP): go.mod go.sum
	go mod vendor
	cd $(NWEP_VENDOR) && bash setup.sh
	@touch $@

clean:
	rm -rf vendor
