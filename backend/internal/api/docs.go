package api

import _ "embed"

//go:embed openapi.gen.yaml
var specYAML []byte

// docsHTML renders the contract with Scalar, loaded from its CDN.
const docsHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Tenfold API</title>
  </head>
  <body>
    <div id="app"></div>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
      Scalar.createApiReference('#app', { url: '/openapi.yaml' })
    </script>
  </body>
</html>
`

// Spec returns the OpenAPI contract, as YAML.
func Spec() []byte { return specYAML }

// DocsPage returns the HTML page that renders the contract.
func DocsPage() []byte { return []byte(docsHTML) }
