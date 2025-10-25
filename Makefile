run:
	cd cmd && go run .

vulncheck:
	govulncheck ./...

test:
	go test -race -cover ./...

check-fromat:
	gofumpt -l .

format:
	gofumpt -l -w .

check-lint:
	golangci-lint run ./...