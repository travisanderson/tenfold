package api

// The spec is copied next to this file so docs.go can embed it; go:embed
// cannot reach outside the module.
//go:generate cp ../../../api/openapi.yaml openapi.gen.yaml
//go:generate go tool oapi-codegen -config oapi.cfg.yaml ../../../api/openapi.yaml
