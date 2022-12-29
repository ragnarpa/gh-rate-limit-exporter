BIN := gh-rate-limit-exporter
COVERFILE := cover.out

.PHONY: all
all: build test

.PHONY: build
build:
	CGO_ENABLED=0 go build -o $(BIN) main.go

.PHONY: test
test:
	go test ./... -race -v -coverprofile $(COVERFILE)

.PHONY: run
run:
	go run main.go
 
clean:
	go clean
	rm -fr $(BIN) $(COVERFILE)