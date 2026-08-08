.PHONY: dev-up dev-down run-api run-models proto test test-int build up down logs clean

dev-up:
	docker compose -f docker-compose.dev.yml up -d

dev-down:
	docker compose -f docker-compose.dev.yml down

run-api:
	go run ./cmd/server

run-models:
	cd model_service && .venv/bin/python service.py

proto:
	python -m grpc_tools.protoc \
		-I proto/models \
		--python_out=gen/models \
		--grpc_python_out=gen/models \
		--pyi_out=gen/models \
		proto/models/models.proto
	protoc \
		--go_out=gen/go/models \
		--go_opt=paths=source_relative \
		--go-grpc_out=gen/go/models \
		--go-grpc_opt=paths=source_relative \
		-I proto/models \
		proto/models/models.proto

test:
	go test -race ./internal/...

test-int:
	go test ./integration/... -tags integration -v

build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

logs-%:
	docker compose logs -f $*

clean:
	docker compose down -v --rmi local
	docker compose -f docker-compose.dev.yml down -v --rmi local