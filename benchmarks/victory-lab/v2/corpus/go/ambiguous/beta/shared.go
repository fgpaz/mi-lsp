package beta

// Shared intentionally has a same-named peer in package alpha.
type Shared struct{}

func New() Shared {
	return Shared{}
}
