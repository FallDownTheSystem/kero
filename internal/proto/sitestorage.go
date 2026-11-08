package proto

import "encoding/json"

type (
	// SiteStorage is a site's localStorage and IndexedDB, as kero browser
	// cookies import took them of the user's own browser: a page of the
	// site's origin in the app writes them for the browser panes.
	SiteStorage struct {
		// Origin is the site's, as a page's location.origin.
		Origin       string        `json:"origin"`
		LocalStorage []StorageItem `json:"localStorage,omitempty"`
		// IndexedDB is the site's databases, as the import's scripts read
		// and write them (internal/localbrowser), which kerod passes on as
		// they are.
		IndexedDB json.RawMessage `json:"indexedDB,omitempty"`
	}
	// StorageItem is an item of localStorage, as Playwright's storage
	// state has it.
	StorageItem struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
)
