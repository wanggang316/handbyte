.PHONY: build install run tidy clean

BIN := handbyte

build:
	go build -o $(BIN) .

install: build
	install -m 0755 $(BIN) $(HOME)/.local/bin/$(BIN)

run:
	go run .

tidy:
	go mod tidy

clean:
	rm -f $(BIN)
