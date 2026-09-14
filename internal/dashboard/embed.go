package dashboard

import "embed"

// Assets contains the built dashboard; build it with npm run build in ui.
//
//go:embed assets
var Assets embed.FS
