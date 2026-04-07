.PHONY: build build-pam run clean hash-password

build:
	go build -o liteterm ./cmd/liteterm

build-pam:
	go build -tags pam -o liteterm ./cmd/liteterm

run: build
	./liteterm --port 3000

clean:
	rm -f liteterm

hash-password: build
	@echo "Enter password (then press Ctrl+D):"
	@./liteterm --password-hash
