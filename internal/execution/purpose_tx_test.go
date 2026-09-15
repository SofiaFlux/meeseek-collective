package execution

import (
	"database/sql"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

func TestPurposeValidationCanShareTaskCreationTransaction(t *testing.T) {
	svc, _, ctx, missionID := newExecutionService(t)
	err := svc.store.WithTx(ctx, func(tx *sql.Tx) error {
		return svc.purpose.ValidatePurposeTx(ctx, tx, domain.PurposeRef{
			Kind: domain.PurposeMission,
			ID:   missionID,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}
