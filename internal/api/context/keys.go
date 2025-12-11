package context

type Key string

const (
	Claims Key = "claims"
	Tenant Key = "tenant"
	Params Key = "params"
)
