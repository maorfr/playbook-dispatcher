generate:
	~/go/bin/oapi-codegen -generate server,spec -package controllers -o internal/api/controllers/spec.gen.go schema/api.spec.yaml
	~/go/bin/oapi-codegen -generate types -package controllers -o internal/api/controllers/types.gen.go schema/api.spec.yaml
	~/go/bin/oapi-codegen -generate client,types -package tests -o internal/api/tests/client.gen.go schema/api.spec.yaml

build:
	go build cmd/pd/pd.go

migrate-db:
	./migrate.linux-amd64 -source file://./migrations -database postgresql://insights:insights@${DB_HOST}:5432/insights?sslmode=disable up

migrate-db-undo:
	./migrate.linux-amd64 -source file://./migrations -database postgresql://insights:insights@${DB_HOST}:5432/insights?sslmode=disable down 2

run: migrate-db
	ACG_CONFIG=$(shell pwd)/cdappconfig.json go run cmd/pd/main.go

test: migrate-db
	ACG_CONFIG=$(shell pwd)/cdappconfig.json go test -p 1 -v ./...