.PHONY: build install clean

build:
	go build -v -o build/claude-squad

install: build
	cp build/claude-squad /opt/homebrew/bin/claude-squad

clean:
	rm -rf build/
