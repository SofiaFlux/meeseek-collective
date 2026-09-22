package identity

import (
	"crypto/ed25519"

	"github.com/SofiaFlux/summa42/internal/domain"
)

const LocalDevFileCustody = "LOCAL_DEV_FILE"

type Signer interface {
	PrincipalID() domain.ID
	PublicKey() ed25519.PublicKey
	Sign(message []byte) ([]byte, error)
	CustodyProfile() string
}
