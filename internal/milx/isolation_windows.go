//go:build windows

package milx

func NewPlatformIsolationGuard() (*IsolationGuard, error) {
	return nil, NewError("GPH_MILX_NETWORK_FORBIDDEN", "isolation", false, "", "platform isolation is not enforced")
}
