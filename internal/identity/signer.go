package identity

import (
	"crypto/ed25519"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

const LocalDevFileCustody = "LOCAL_DEV_FILE"

type Signer interface {
	PrincipalID() domain.ID
	PublicKey() ed25519.PublicKey
	Sign(message []byte) ([]byte, error)
	CustodyProfile() string
}
