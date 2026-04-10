VERSION ?= $(shell git describe --tags 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/dvflw/mantle/internal/version.Version=$(VERSION) \
           -X github.com/dvflw/mantle/internal/version.Commit=$(COMMIT) \
           -X github.com/dvflw/mantle/internal/version.Date=$(DATE)

.PHONY: build test lint clean migrate run dev spec spec-patch

build:
	go build -ldflags "$(LDFLAGS)" -o mantle ./cmd/mantle

test:
	go test -race ./...

lint:
	golangci-lint run

clean:
	rm -f mantle

migrate:
	go run ./cmd/mantle init

run:
	go run ./cmd/mantle $(ARGS)

dev:
	docker-compose up -d

spec: ## Regenerate OpenAPI spec (requires: go install github.com/swaggo/swag/cmd/swag@v1.16.6)
	swag init \
	  -g docs.go \
	  --dir internal/server,internal/workflow,internal/budget \
	  --output internal/server/docs \
	  --outputTypes json,yaml \
	  --requiredByDefault \
	  --parseInternal
	$(MAKE) spec-patch

spec-patch: ## Re-apply manual OpenAPI patches lost on swag regeneration
	@# swag init does not emit global security declarations, per-operation security
	@# overrides, additionalProperties on free-form objects, or RFC 3339/date format
	@# annotations on timestamp fields. Patch them back after every regeneration run.
	@# Requires jq and yq (pip install yq).
	jq --indent 4 --ascii-output '.security=[{"ApiKeyAuth":[]},{"OIDCAuth":[]}]|.paths["/healthz"].get.security=[]|.paths["/readyz"].get.security=[]|.definitions["server.WorkflowDetailResponse"].properties.definition.additionalProperties=true|.definitions["server.UsageResponse"].properties.period_start.format="date"|.definitions["server.ExecutionDetail"].properties.started_at.format="date-time"|.definitions["server.ExecutionDetail"].properties.completed_at.format="date-time"|.definitions["server.ExecutionSummary"].properties.started_at.format="date-time"|.definitions["server.ExecutionSummary"].properties.completed_at.format="date-time"|.definitions["server.StepSummary"].properties.started_at.format="date-time"|.definitions["server.StepSummary"].properties.completed_at.format="date-time"|.definitions["workflow.VersionSummary"].properties.created_at.format="date-time"|.definitions["workflow.WorkflowSummary"].properties.created_at.format="date-time"|.definitions["workflow.WorkflowSummary"].properties.updated_at.format="date-time"' \
	  internal/server/docs/swagger.json > internal/server/docs/swagger.json.tmp && \
	  mv internal/server/docs/swagger.json.tmp internal/server/docs/swagger.json
	yq -y '.security=[{"ApiKeyAuth":[]},{"OIDCAuth":[]}]|.paths["/healthz"].get.security=[]|.paths["/readyz"].get.security=[]|.definitions["server.WorkflowDetailResponse"].properties.definition.additionalProperties=true|.definitions["server.UsageResponse"].properties.period_start.format="date"|.definitions["server.ExecutionDetail"].properties.started_at.format="date-time"|.definitions["server.ExecutionDetail"].properties.completed_at.format="date-time"|.definitions["server.ExecutionSummary"].properties.started_at.format="date-time"|.definitions["server.ExecutionSummary"].properties.completed_at.format="date-time"|.definitions["server.StepSummary"].properties.started_at.format="date-time"|.definitions["server.StepSummary"].properties.completed_at.format="date-time"|.definitions["workflow.VersionSummary"].properties.created_at.format="date-time"|.definitions["workflow.WorkflowSummary"].properties.created_at.format="date-time"|.definitions["workflow.WorkflowSummary"].properties.updated_at.format="date-time"' \
	  internal/server/docs/swagger.yaml > internal/server/docs/swagger.yaml.tmp && \
	  mv internal/server/docs/swagger.yaml.tmp internal/server/docs/swagger.yaml
