package alpha

// Shared intentionally has a same-named peer in package beta.
type Shared struct{}

func New() Shared {
	return Shared{}
}
