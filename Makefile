APP_NAME := kube-query
GO_FILES := main.go
OUTPUT := bin/$(APP_NAME)
DB_FILE := out/kube_data.db
define RESOURCES
rhacs:deployment:fleetshard-sync
endef
export RESOURCES

# Commands
.PHONY: all build run clean db-clean help

all: build

build:
	@echo "Building $(APP_NAME)..."
	go build -o $(OUTPUT) $(GO_FILES)

run: build
	@echo "Running $(APP_NAME)..."
	export RESOURCES
	./$(OUTPUT) --db $(DB_FILE) --resources $(RESOURCES)

clean:
	@echo "Cleaning up..."
	rm -f $(OUTPUT)

db-clean:
	@echo "Removing database file..."
	rm -f $(DB_FILE)
