package netenforce

import provision "github.com/crispuscrew/zinc/common/adapters/network"

// LookupFunc takes the app's explicit resolver configuration. The zero value
// refuses domain rules; it never uses the host's ambient resolver.
type LookupFunc = provision.Lookup
