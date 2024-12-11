APP_NAME := kube-query
GO_FILES := main.go
OUTPUT := $(APP_NAME)
DB_FILE := kube_data.db
RESOURCES := "default:deployment:example-deployment,default:configmap:example-configmap,default:secret:example-secret"
KUBECONFIG := $(HOME)/.kube/config

# Commands
.PHONY: all build run clean db-clean help

all: build

build:
	@echo "Building $(APP_NAME)..."
	go build -o $(OUTPUT) $(GO_FILES)

run: build
	@echo "Running $(APP_NAME)..."
	./$(OUTPUT) --db $(DB_FILE) --resources $(RESOURCES)

clean:
	@echo "Cleaning up..."
	rm -f $(OUTPUT)

db-clean:
	@echo "Removing database file..."
	rm -f $(DB_FILE)
