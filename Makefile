BIN := bin
LDFLAGS := -s -w

.PHONY: all test build pi pi32 run sim clean

all: test build

test:
	go vet ./...
	go test -race ./...

build:
	go build -ldflags '$(LDFLAGS)' -o $(BIN)/gpstrack-gateway ./cmd/gateway
	go build -ldflags '$(LDFLAGS)' -o $(BIN)/gpstrack-sim ./cmd/sim

# Raspberry Pi 4/5 running 64-bit Raspberry Pi OS
pi:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BIN)/arm64/gpstrack-gateway ./cmd/gateway
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BIN)/arm64/gpstrack-sim ./cmd/sim

# Older Pis or 32-bit Raspberry Pi OS
pi32:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BIN)/armv7/gpstrack-gateway ./cmd/gateway
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BIN)/armv7/gpstrack-sim ./cmd/sim

run:
	go run ./cmd/gateway

sim:
	go run ./cmd/sim -every 2s

clean:
	rm -rf $(BIN)
