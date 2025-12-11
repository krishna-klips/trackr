package redirect

import (
	"sync"
	"time"
	"trackr/internal/engine/links"
)

type CachedLink struct {
	ID             string
	DestinationURL string
	Rules          *links.RedirectRules
	RedirectType   string
	Status         string
	CachedAt       time.Time
}

type LinkCache struct {
	store sync.Map // map[short_code]*CachedLink
	ttl   time.Duration
}

func NewLinkCache(ttl time.Duration) *LinkCache {
	return &LinkCache{
		ttl: ttl,
	}
}

func (c *LinkCache) Get(shortCode string) (*CachedLink, bool) {
	val, ok := c.store.Load(shortCode)
	if !ok {
		return nil, false
	}

	link := val.(*CachedLink)
	if time.Since(link.CachedAt) > c.ttl {
		c.store.Delete(shortCode)
		return nil, false
	}

	return link, true
}

func (c *LinkCache) Set(shortCode string, link *links.Link) {
	cached := &CachedLink{
		ID:             link.ID,
		DestinationURL: link.DestinationURL,
		Rules:          link.Rules,
		RedirectType:   link.RedirectType,
		Status:         link.Status,
		CachedAt:       time.Now(),
	}
	c.store.Store(shortCode, cached)
}
