package core

import (
	"fmt"
	"slices"

	"golang.org/x/crypto/ssh"
)

// SF-C03: append only the algorithms in OShell 0.2.92 Models.swift. In
// particular, x/crypto's RC4 and DSA lists are not the compatibility contract.
func configureLegacySSH(cfg *ssh.ClientConfig) {
	good, available := ssh.SupportedAlgorithms(), ssh.InsecureAlgorithms()
	appendSupported := func(base, extra, supported []string) []string {
		out := slices.Clone(base)
		for _, value := range extra {
			if !slices.Contains(out, value) && slices.Contains(supported, value) {
				out = append(out, value)
			}
		}
		return out
	}
	cfg.KeyExchanges = appendSupported(good.KeyExchanges, []string{"diffie-hellman-group14-sha1", "diffie-hellman-group-exchange-sha1", "diffie-hellman-group1-sha1"}, available.KeyExchanges)
	cfg.Ciphers = appendSupported(good.Ciphers, []string{"aes128-cbc", "aes192-cbc", "aes256-cbc", "3des-cbc"}, available.Ciphers)
	cfg.MACs = appendSupported(good.MACs, []string{"hmac-sha1", "hmac-sha1-96"}, available.MACs)
	cfg.HostKeyAlgorithms = appendSupported(good.HostKeys, []string{"ssh-rsa"}, available.HostKeys)
}

func restrictRSAAuthentication(signers []ssh.Signer, legacy bool) ([]ssh.Signer, error) {
	result := slices.Clone(signers)
	for i, signer := range result {
		if signer.PublicKey().Type() != ssh.KeyAlgoRSA && signer.PublicKey().Type() != ssh.CertAlgoRSAv01 {
			continue
		}
		algorithmSigner, ok := signer.(ssh.AlgorithmSigner)
		if !ok {
			return nil, fmt.Errorf("RSA 签名器不支持显式算法选择")
		}
		algorithms := []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
		if legacy {
			algorithms = append(algorithms, ssh.KeyAlgoRSA)
		}
		if multi, ok := signer.(ssh.MultiAlgorithmSigner); ok {
			algorithms = slices.DeleteFunc(algorithms, func(s string) bool { return !slices.Contains(multi.Algorithms(), s) })
		}
		limited, err := ssh.NewSignerWithAlgorithms(algorithmSigner, algorithms)
		if err != nil {
			return nil, err
		}
		result[i] = limited
	}
	return result, nil
}
