.PHONY: build clean run test

build:
	go build -o bin/preprocess ./cmd/preprocess

run: build
	./bin/preprocess

clean:
	rm -rf bin/ output/ .journal/

test:
	go test ./...
