package geoip

// Resolver defines the interface for GeoIP lookups
type Resolver interface {
	Lookup(ip string) (string, error)
	LookupCity(ip string) (string, error)
}

// DummyResolver is a placeholder for when the MaxMind DB is not available
type DummyResolver struct{}

func NewDummyResolver() *DummyResolver {
	return &DummyResolver{}
}

func (r *DummyResolver) Lookup(ip string) (string, error) {
	// Return default or mock
	return "US", nil
}

func (r *DummyResolver) LookupCity(ip string) (string, error) {
	return "New York", nil
}
