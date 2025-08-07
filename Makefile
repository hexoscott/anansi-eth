fmt:
	golangci-lint run && treefmt

.PHONY: contracts
contracts:
	cd contracts && forge build --force