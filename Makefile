.PHONY: build build-pam run clean hash-password

build:
	go build -o roambench ./cmd/roambench

build-pam:
	go build -tags pam -o roambench ./cmd/roambench

run: build
	./roambench --port 3000

clean:
	rm -f roambench

hash-password: build
	@echo "Enter password (then press Ctrl+D):"
	@./roambench --password-hash
