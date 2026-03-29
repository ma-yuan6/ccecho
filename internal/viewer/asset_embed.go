package viewer

import "embed"

//go:embed assets/index.html assets/style.css assets/app.js
var assetFS embed.FS
