package app

import legacyserver "agp/backend/internal/server"

// Run starts the HTTP API service.
//
// The current server package is kept as a compatibility adapter while domain
// modules are migrated behind explicit handler/service/repository boundaries.
func Run() error {
	return legacyserver.Run()
}
