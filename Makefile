.PHONY: clean
# Default tag and architecture. Can be overridden
TAG?=$(shell git describe --tags --dirty)
ARCH?=amd64
# Only enable CGO (and build the UDP backend) on AMD64
ifeq ($(ARCH),amd64)
	CGO_ENABLED=1
else
	CGO_ENABLED=0
endif

# Go version to use for builds
GO_VERSION=1.11.1

# K8s version used for Makefile helpers
K8S_VERSION=v1.12.0

GOARM=7

### BUILDING
clean:
	rm -f build/fledged*
	rm -f dist/*.tar.gz
	docker image rm senthilrch/fledged:latest

fledged: 
	CGO_ENABLED=0 go build -o build/fledged \
	  -ldflags '-s -w -extldflags "-static"' cmd/fledged.go

image: clean fledged
	cd build && docker build -t "senthilrch/fledged:latest" . && \
    docker save -o fledged-latest.tar senthilrch/fledged:latest && \
	gzip fledged-latest.tar && docker push senthilrch/fledged:latest

rollout:
	kubectl scale deployment fledged --replicas=0 && sleep 5 && \
	kubectl scale deployment fledged --replicas=1 && sleep 5 && \
	kubectl get pods -l run=fledged