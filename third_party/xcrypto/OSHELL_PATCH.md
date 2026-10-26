# OShell SF-C03 compatibility patch

This directory copies golang.org/x/crypto v0.57.0, with original file hashes in
upstream.json and the unchanged BSD license in LICENSE. ssh/cipher.go and
ssh/common.go register aes192-cbc and aes256-cbc
with 24/32-byte keys using the existing AES-CBC packet implementation and AES
rekey limits. They are listed only in InsecureAlgorithms, never defaults or
SupportedAlgorithms, and remain unavailable in FIPS mode.

Desktop enables them only via the existing LegacySSH switch and fixed OShell
algorithm allowlist. Native OpenSSH interoperability exposed an existing CBC
bug: the upstream CBC path ignored the negotiated EtM MAC. cipher_cbc_etm.go
implements the framing in OpenSSH PROTOCOL section 1.5, authenticating the clear
length and encrypted body before decrypting any bytes. It uses the existing
AES/CBC and HMAC primitives, with bounded lengths and padding checks. Non-EtM
packet framing and its existing oracle mitigation, key derivation, supported
modern algorithms and defaults are unchanged.

Local tests cover all CBC/MAC pairs, corruption without decryption, malformed
lengths and non-EtM oracle mitigation. Application tests exercise default
rejection, opt-in negotiation, multi-packet traffic and native OpenSSH with
SHA256/SHA512 EtM and SHA256/SHA1-96 MACs. This remains SF-C03, not a new algorithm
set or SSH protocol feature.
