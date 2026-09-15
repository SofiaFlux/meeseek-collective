package domain

import (
	"crypto/rand"
	"encoding/hex"
)

type ID string

func NewID(prefix string) ID {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return ID(prefix + "_" + hex.EncodeToString(b[:]))
}
