# The contract in api/openapi.yaml is the source of truth.
# Go and TypeScript types are generated from it and committed.

GO ?= go

GENERATED := \
	backend/internal/api/api.gen.go \
	backend/internal/api/openapi.gen.yaml \
	frontend/src/lib/api/schema.d.ts

.PHONY: lint generate generate-go generate-ts check-drift samples holdout holdout-sweep test

lint:
	npm run -s lint:api

generate: lint generate-go generate-ts

generate-go:
	cd backend && $(GO) generate ./...

generate-ts:
	npm run -s generate:ts

# Writes the demo logs and their answer keys. Same seed, same bytes.
# The logs are kept out of version control; the generator is tracked.
samples:
	cd samples && $(GO) run ./loggen --all --seed 42

# Writes a day the detectors were not built against: the same scenarios,
# with users, times, sizes, hosts and intervals drawn from the seed. It
# lives in its own folder, apart from the demos. Score it with
# `cd backend && go test ./internal/detect -run TestHoldout -v`.
holdout:
	cd samples && $(GO) run ./loggen --profile holdout --seed 20261012 --out holdout

# Scores the detectors on holdout seeds 101 to 150 and rewrites
# backend/eval/holdout-sweep.txt, the source of the numbers in
# backend/README.md. No model is called.
holdout-sweep:
	scripts/holdout-sweep.sh

test:
	cd backend && $(GO) test ./...

# Fails if the committed generated files do not match the spec.
check-drift: generate
	@if [ -n "$$(git status --porcelain -- $(GENERATED))" ]; then \
		echo "Generated code is out of date. Run 'make generate' and commit:"; \
		git status --short -- $(GENERATED); \
		exit 1; \
	fi
	@echo "Generated code matches the spec."
