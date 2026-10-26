package ssh

import (
	"bytes"
	"crypto"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"strings"
	"testing"
)

type rejectDecrypt struct{ cipher.BlockMode }

func (r rejectDecrypt) CryptBlocks(dst, src []byte) { panic("unauthenticated ciphertext decrypted") }

func TestOShellCBCMACMatrix(t *testing.T) {
	for _, name := range []string{InsecureCipherAES128CBC, InsecureCipherAES192CBC, InsecureCipherAES256CBC, InsecureCipherTripleDESCBC} {
		for mac := range macModes {
			t.Run(name+"/"+mac, func(t *testing.T) { testPacketCipher(t, name, mac) })
		}
	}
}

func TestOShellCBCAuthenticateBeforeDecrypt(t *testing.T) {
	for _, name := range []string{InsecureCipherAES192CBC, InsecureCipherAES256CBC} {
		for _, mac := range []string{HMACSHA256ETM, HMACSHA512ETM} {
			t.Run(name+"/"+mac, func(t *testing.T) {
				algs := DirectionAlgorithms{Cipher: name, MAC: mac, compression: compressionNone}
				newCipher := func() *cbcCipher {
					p, err := newPacketCipher(clientKeys, algs, &kexResult{Hash: crypto.SHA256})
					if err != nil {
						t.Fatal(err)
					}
					return p.(*cbcCipher)
				}
				writer := newCipher()
				var wire bytes.Buffer
				if err := writer.writeCipherPacket(7, &wire, rand.Reader, []byte("synthetic-payload")); err != nil {
					t.Fatal(err)
				}
				original := wire.Bytes()
				for i := 4; i < len(original); i++ {
					bad := bytes.Clone(original)
					bad[i] ^= 1
					reader := newCipher()
					reader.decrypter = rejectDecrypt{reader.decrypter}
					if _, err := reader.readCipherPacket(7, bytes.NewReader(bad)); err == nil || !strings.Contains(err.Error(), "MAC") {
						t.Fatal("tamper accepted", i, err)
					}
				}
				for _, n := range []uint32{0, 15, 17, maxPacket + 1, 0xffffffff} {
					bad := bytes.Clone(original)
					binary.BigEndian.PutUint32(bad, n)
					reader := newCipher()
					reader.decrypter = rejectDecrypt{reader.decrypter}
					if _, err := reader.readCipherPacket(7, bytes.NewReader(bad)); err == nil {
						t.Fatal("bad length accepted", n)
					}
				}
				reader := newCipher()
				reader.decrypter = rejectDecrypt{reader.decrypter}
				if _, err := reader.readCipherPacket(8, bytes.NewReader(original)); err == nil {
					t.Fatal("wrong sequence accepted")
				}
			})
		}
	}
}
