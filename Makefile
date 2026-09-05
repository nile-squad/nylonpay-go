# Runs the tests
.PHONY: test
test:
	@echo "Starting test execution..."
	@GO_ENV=testing go test ./... -v

# Runs the tests with the race detector
.PHONY: test-race
test-race:
	@echo "Starting test execution with the race detector..."
	@GO_ENV=testing go test -race ./... -v

# Runs integration tests
.PHONY: test-integration
test-integration:
	@echo "Starting integration test execution..."
	@GO_ENV=testing go test -tags=integration ./tests/integration/ -v

# Runs the S19 test in the crypto package
.PHONY: test-crypto
test-crypto:
	@echo "Running S19 crypto test..."
	@GO_ENV=testing go test -run 'S19' ./internal/crypto/ -v



