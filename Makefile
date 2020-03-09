.PHONY: fmt test vet install package
all: test vet install

fmt:
	go fmt ./...

test:
	go test -short ./...

vet:
	go vet ./...

install:
	go install ./...

package:
	docker run --rm -v $(PWD):/repo -v $(PWD)/build:/build mullvadvpn/go-packager@sha256:841311ae78ae85e2e530eac40c76d7509f74e8c16818c87933bb3ce3e83eff8c
