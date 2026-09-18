PREFIX ?= $(HOME)/.local

.PHONY: build test install uninstall

build:
	go build -o bin/bev .

test:
	go test ./...
	zsh -f tests/shell.zsh
	python3 tests/terminal.py

install: build
	install -d "$(PREFIX)/bin" "$(PREFIX)/share/bev"
	install -m 755 bin/bev "$(PREFIX)/bin/bev"
	install -m 644 shell/bev.zsh "$(PREFIX)/share/bev/bev.zsh"

uninstall:
	rm -f "$(PREFIX)/bin/bev" "$(PREFIX)/share/bev/bev.zsh"
