.PHONY: build install uninstall run tidy clean

BIN := handbyte
PREFIX ?= $(HOME)/.local

build:
	go build -o $(BIN) .

install: build
	install -m 0755 $(BIN) $(PREFIX)/bin/$(BIN)

uninstall:
	rm -f $(PREFIX)/bin/$(BIN)

run:
	go run .

tidy:
	go mod tidy

clean:
	rm -f $(BIN)
