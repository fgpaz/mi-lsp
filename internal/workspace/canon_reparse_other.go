//go:build !windows

package workspace

func canonPathIsReparse(_ string) (bool, error) {
	return false, nil
}
