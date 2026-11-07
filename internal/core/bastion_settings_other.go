//go:build !windows

package core

func applyUSMPuTTYSettings(*TerminalLaunchRequest) {}

func ssoToolPath([]string) string { return "" }
func readSSOToolSession(string) (TerminalLaunchRequest, error) {
	return TerminalLaunchRequest{}, errBastionArgs
}
