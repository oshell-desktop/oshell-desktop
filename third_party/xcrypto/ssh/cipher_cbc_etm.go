// Copyright 2026 OShell Desktop contributors.
// Use of this source code is governed by the BSD-style LICENSE file.

package ssh

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
)

// SF-C03: the same negotiated EtM MACs already used by the SSH package must
// also work with CBC. OpenSSH PROTOCOL section 1.5 leaves the four-byte length
// in cleartext, then authenticates seq || length || encrypted body. Do not
// decrypt even the padding-length byte before authenticating the whole body.
func (c *cbcCipher) readEtMPacket(seq uint32, r io.Reader) ([]byte, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(prefix[:])
	block := maxUInt32(cbcMinPacketSizeMultiple, c.decrypter.BlockSize())
	if n < cbcMinPacketSize || n > maxPacket || n%block != 0 {
		return nil, errors.New("ssh: invalid EtM CBC packet length")
	}
	total := 4 + n + c.macSize
	if uint32(cap(c.packetData)) < total {
		c.packetData = make([]byte, total)
	} else {
		c.packetData = c.packetData[:total]
	}
	copy(c.packetData, prefix[:])
	if _, err := io.ReadFull(r, c.packetData[4:]); err != nil {
		return nil, err
	}
	c.mac.Reset()
	binary.BigEndian.PutUint32(c.seqNumBytes[:], seq)
	c.mac.Write(c.seqNumBytes[:])
	c.mac.Write(c.packetData[:4+n])
	c.macResult = c.mac.Sum(c.macResult[:0])
	if subtle.ConstantTimeCompare(c.macResult, c.packetData[4+n:]) != 1 {
		return nil, errors.New("ssh: MAC failure")
	}
	body := c.packetData[4 : 4+n]
	c.decrypter.CryptBlocks(body, body)
	padding := uint32(body[0])
	if padding < cbcMinPaddingSize || n <= padding+1 {
		return nil, errors.New("ssh: invalid EtM CBC padding")
	}
	return body[1 : n-padding], nil
}

func (c *cbcCipher) writeEtMPacket(seq uint32, w io.Writer, random io.Reader, packet []byte) error {
	if len(packet) > maxPacket-cbcMinPaddingSize-1 {
		return errors.New("ssh: packet too large")
	}
	block := int(maxUInt32(cbcMinPacketSizeMultiple, c.encrypter.BlockSize()))
	n := int(maxUInt32(1+len(packet)+cbcMinPaddingSize, cbcMinPacketSize))
	n = (n + block - 1) / block * block
	if n > maxPacket {
		return errors.New("ssh: packet too large")
	}
	padding := n - 1 - len(packet)
	total := 4 + n + int(c.macSize)
	if cap(c.packetData) < total {
		c.packetData = make([]byte, 4+n, total)
	} else {
		c.packetData = c.packetData[:4+n]
	}
	binary.BigEndian.PutUint32(c.packetData[:4], uint32(n))
	body := c.packetData[4:]
	body[0] = byte(padding)
	copy(body[1:], packet)
	if _, err := io.ReadFull(random, body[1+len(packet):]); err != nil {
		return err
	}
	c.encrypter.CryptBlocks(body, body)
	c.mac.Reset()
	binary.BigEndian.PutUint32(c.seqNumBytes[:], seq)
	c.mac.Write(c.seqNumBytes[:])
	c.mac.Write(c.packetData)
	c.packetData = c.mac.Sum(c.packetData)
	_, err := w.Write(c.packetData)
	return err
}
