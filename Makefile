.PHONY: dev-up dev-down run-api run-models proto proto-models proto-crawler \
        test test-int build up down logs clean spider-build spider-test

# dev compose (Redis + SearXNG only, no built images) 
dev-up:
	docker compose -f docker-compose.dev.yml up -d

dev-down:
	docker compose -f docker-compose.dev.yml down

# local run 
run-api:
	go run ./cmd/server

run-models:
	cd model_service && .venv/bin/python service.py

run-spider:
	cd spider && RUST_LOG=info cargo run --release

# proto generation
proto: proto-models proto-crawler

proto-models:
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

proto-crawler:
	protoc \
		--go_out=gen/go/crawler \
		--go_opt=paths=source_relative \
		--go-grpc_out=gen/go/crawler \
		--go-grpc_opt=paths=source_relative \
		-I proto/crawler \
		proto/crawler/crawler.proto

# testing 
test:
	go test -race ./internal/...

test-int:
	go test -race ./integration/... -tags integration -v

spider-test:
	cd spider && cargo test

# docker
spider-build:
	docker build -f spider/Dockerfile -t opensearch-spider:dev .

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
	cd spider && cargo clean