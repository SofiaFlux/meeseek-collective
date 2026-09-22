package operations

import (
	"context"
	"errors"
	"strings"

	"github.com/SofiaFlux/summa42/internal/domain"
)

func (s *Service) Operation(ctx context.Context, operationID domain.ID) (domain.ExternalOperation, error) {
	if err := s.configured(); err != nil {
		return domain.ExternalOperation{}, err
	}
	operationID = domain.ID(strings.TrimSpace(string(operationID)))
	if operationID == "" {
		return domain.ExternalOperation{}, errors.New("operation id is required")
	}
	return s.loadOperation(ctx, operationID)
}
