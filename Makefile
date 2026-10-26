.PHONY: build install release test clean

build:
	go build -ldflags="-s -w" -o val2harness .

install: build
	cp val2harness ~/.local/bin/val2harness
	@echo "[OK] Installed val2harness globally to ~/.local/bin/val2harness"

test:
	go test ./...

clean:
	rm -f val2harness
	rm -rf dist/
