API_DIR := ../../scripts/organizer

.PHONY: test build run migrate

test:
	go test ./$(API_DIR)/...

build:
	go build -o agenda-api.exe ./$(API_DIR)

run:
	go run ./$(API_DIR)

migrate:
	go test ./$(API_DIR)/... -run 'Test(LegacyDatabaseGetsDefaultWorkspace|MigrationIsIdempotent)' -count=1
