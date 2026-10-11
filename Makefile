# The contract in api/openapi.yaml is the source of truth.
# Go and TypeScript types are generated from it and committed.

GO ?= go

GENERATED := \
	backend/internal/api/api.gen.go \
	backend/internal/api/openapi.gen.yaml \
	frontend/src/lib/api/schema.d.ts

.PHONY: lint generate generate-go generate-ts check-drift

lint:
	npm run -s lint:api

generate: lint generate-go generate-ts

generate-go:
	cd backend && $(GO) generate ./...

generate-ts:
	npm run -s generate:ts

# Fails if the committed generated files do not match the spec.
check-drift: generate
	@if [ -n "$$(git status --porcelain -- $(GENERATED))" ]; then \
		echo "Generated code is out of date. Run 'make generate' and commit:"; \
		git status --short -- $(GENERATED); \
		exit 1; \
	fi
	@echo "Generated code matches the spec."
