package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/clock"
	"github.com/SofiaFlux/summa42/internal/domain"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
)

type EvidenceInput struct {
	Ref  domain.EvidenceRef
	Kind string
}

type ClaimInput struct {
	ID          domain.ID
	SubjectID   domain.ID
	Predicate   string
	Statement   string
	Status      domain.ClaimStatus
	Confidence  float64
	ValidFrom   time.Time
	ValidTo     time.Time
	EvidenceIDs []domain.ID
	SourceID    domain.ID
}

type KnowledgeDelta struct {
	ID       domain.ID
	SourceID domain.ID
	Evidence []EvidenceInput
	Claims   []ClaimInput
}

type Service struct {
	store *state.Store
	clock clock.Clock
}

func New(store *state.Store, clk clock.Clock) *Service {
	return &Service{store: store, clock: clk}
}

func (s *Service) PutEvidence(ctx context.Context, input EvidenceInput) error {
	if err := s.configured(); err != nil {
		return err
	}
	input, err := s.validateEvidence(input)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		return s.putEvidenceTx(ctx, tx, input)
	})
}

func (s *Service) AddClaim(ctx context.Context, input ClaimInput) (domain.Claim, error) {
	if err := s.configured(); err != nil {
		return domain.Claim{}, err
	}
	input, err := validateClaimInput(input)
	if err != nil {
		return domain.Claim{}, err
	}
	var claim domain.Claim
	err = s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		claim, err = s.addClaimTx(ctx, tx, input)
		return err
	})
	return claim, err
}

func (s *Service) Ingest(ctx context.Context, delta KnowledgeDelta) ([]domain.Claim, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	delta.ID = domain.ID(strings.TrimSpace(string(delta.ID)))
	delta.SourceID = domain.ID(strings.TrimSpace(string(delta.SourceID)))
	if delta.ID == "" {
		return nil, errors.New("knowledge delta id is required")
	}
	if delta.SourceID == "" {
		return nil, errors.New("knowledge delta source id is required")
	}
	if len(delta.Claims) == 0 {
		return nil, errors.New("knowledge delta must contain at least one claim")
	}

	validatedEvidence := make([]EvidenceInput, len(delta.Evidence))
	for i, input := range delta.Evidence {
		validated, err := s.validateEvidence(input)
		if err != nil {
			return nil, fmt.Errorf("evidence %d: %w", i, err)
		}
		validatedEvidence[i] = validated
	}
	validatedClaims := make([]ClaimInput, len(delta.Claims))
	for i, input := range delta.Claims {
		input.SourceID = delta.SourceID
		validated, err := validateClaimInput(input)
		if err != nil {
			return nil, fmt.Errorf("claim %d: %w", i, err)
		}
		validatedClaims[i] = validated
	}

	claims := make([]domain.Claim, 0, len(validatedClaims))
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		now := s.clock.Now().UTC()
		if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_deltas(delta_id, source_id, ingested_at) VALUES (?, ?, ?)`, delta.ID, delta.SourceID, formatTime(now)); err != nil {
			return fmt.Errorf("record knowledge delta: %w", err)
		}
		for _, input := range validatedEvidence {
			if err := s.putEvidenceTx(ctx, tx, input); err != nil {
				return err
			}
		}
		for i, input := range validatedClaims {
			claim, err := s.addClaimTx(ctx, tx, input)
			if err != nil {
				return err
			}
			claims = append(claims, claim)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO knowledge_delta_claims(delta_id, claim_id, submitted_status, submitted_confidence)
				VALUES (?, ?, ?, ?)`, delta.ID, claim.ID, input.Status, input.Confidence); err != nil {
				return fmt.Errorf("record knowledge delta claim %d: %w", i, err)
			}
		}
		return s.appendMemoryEventTx(ctx, tx, "KNOWLEDGE_DELTA_INGESTED", delta.ID, map[string]any{
			"source_id": delta.SourceID,
			"claims":    len(claims),
		}, now)
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func (s *Service) Supersede(ctx context.Context, oldClaimID, newClaimID domain.ID, at time.Time) error {
	if err := s.configured(); err != nil {
		return err
	}
	oldClaimID = domain.ID(strings.TrimSpace(string(oldClaimID)))
	newClaimID = domain.ID(strings.TrimSpace(string(newClaimID)))
	if oldClaimID == "" || newClaimID == "" || oldClaimID == newClaimID {
		return errors.New("distinct old and new claim ids are required")
	}
	if at.IsZero() {
		return errors.New("supersession time is required")
	}
	at = at.UTC()
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		oldClaim, err := loadClaim(ctx, tx, oldClaimID)
		if err != nil {
			return err
		}
		if _, err := loadClaim(ctx, tx, newClaimID); err != nil {
			return err
		}
		if at.Before(oldClaim.ValidFrom) {
			return errors.New("supersession cannot precede old claim validity")
		}
		if !oldClaim.ValidTo.IsZero() && oldClaim.ValidTo.Before(at) {
			return errors.New("old claim already ended before supersession")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE claims SET valid_to = ? WHERE claim_id = ?`, formatTime(at), oldClaimID); err != nil {
			return err
		}
		if err := insertRelationTx(ctx, tx, oldClaimID, newClaimID, "SUPERSEDES", at); err != nil {
			return err
		}
		return s.appendMemoryEventTx(ctx, tx, "CLAIM_SUPERSEDED", oldClaimID, map[string]any{"replacement_claim_id": newClaimID}, at)
	})
}

func (s *Service) Claim(ctx context.Context, id domain.ID) (domain.Claim, error) {
	if err := s.configured(); err != nil {
		return domain.Claim{}, err
	}
	return loadClaim(ctx, s.store.DB(), id)
}

func (s *Service) Contradictions(ctx context.Context, id domain.ID) ([]domain.ID, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	rows, err := s.store.DB().QueryContext(ctx, `
		SELECT CASE WHEN left_claim_id = ? THEN right_claim_id ELSE left_claim_id END
		FROM claim_relations
		WHERE kind = 'CONTRADICTS' AND (left_claim_id = ? OR right_claim_id = ?)
		ORDER BY created_at, relation_id`, id, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []domain.ID
	for rows.Next() {
		var other domain.ID
		if err := rows.Scan(&other); err != nil {
			return nil, err
		}
		ids = append(ids, other)
	}
	return ids, rows.Err()
}

func (s *Service) BeliefsAt(ctx context.Context, at time.Time) ([]domain.Claim, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	if at.IsZero() {
		return nil, errors.New("belief query time is required")
	}
	ts := formatTime(at.UTC())
	rows, err := s.store.DB().QueryContext(ctx, `
		SELECT c.claim_id, c.statement, c.status, c.confidence, c.valid_from, c.valid_to, c.created_at
		FROM claims c
		WHERE c.created_at <= ?
		  AND c.valid_from <= ?
		  AND (c.valid_to IS NULL OR c.valid_to > ?)
		  AND EXISTS (SELECT 1 FROM claim_evidence ce WHERE ce.claim_id = c.claim_id)
		  AND NOT EXISTS (
		      SELECT 1
		      FROM claim_evidence ce
		      JOIN evidence_objects e ON e.evidence_id = ce.evidence_id
		      WHERE ce.claim_id = c.claim_id AND e.created_at > ?
		  )
		ORDER BY c.created_at, c.claim_id`, ts, ts, ts, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanClaims(rows)
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]domain.Claim, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is required")
	}
	if limit <= 0 || limit > 100 {
		return nil, errors.New("search limit must be between 1 and 100")
	}
	rows, err := s.store.DB().QueryContext(ctx, `
		SELECT c.claim_id, c.statement, c.status, c.confidence, c.valid_from, c.valid_to, c.created_at
		FROM claim_fts f
		JOIN claims c ON c.claim_id = f.claim_id
		WHERE claim_fts MATCH ?
		ORDER BY rank, c.created_at, c.claim_id
		LIMIT ?`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanClaims(rows)
}

func (s *Service) addClaimTx(ctx context.Context, tx *sql.Tx, input ClaimInput) (domain.Claim, error) {
	for _, evidenceID := range input.EvidenceIDs {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM evidence_objects WHERE evidence_id = ?`, evidenceID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Claim{}, fmt.Errorf("claim evidence %s does not exist", evidenceID)
			}
			return domain.Claim{}, err
		}
	}

	normalized := normalizeStatement(input.Statement)
	validTo := nullableTime(input.ValidTo)
	var existingID domain.ID
	err := tx.QueryRowContext(ctx, `
		SELECT claim_id FROM claims
		WHERE subject_id = ? AND predicate = ? AND normalized_statement = ?
		  AND valid_from = ? AND COALESCE(valid_to, '') = COALESCE(?, '')
		ORDER BY created_at, claim_id LIMIT 1`,
		input.SubjectID, input.Predicate, normalized, formatTime(input.ValidFrom), validTo,
	).Scan(&existingID)
	if err == nil {
		for _, evidenceID := range input.EvidenceIDs {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO claim_evidence(claim_id, evidence_id, linked_at) VALUES (?, ?, ?)`, existingID, evidenceID, formatTime(s.clock.Now().UTC())); err != nil {
				return domain.Claim{}, err
			}
		}
		return loadClaim(ctx, tx, existingID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Claim{}, err
	}

	id := input.ID
	if id == "" {
		id = domain.NewID("claim")
	}
	now := s.clock.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO claims(
			claim_id, subject_id, predicate, statement, normalized_statement, status, confidence,
			valid_from, valid_to, source_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.SubjectID, input.Predicate, input.Statement, normalized, input.Status, input.Confidence,
		formatTime(input.ValidFrom), validTo, input.SourceID, formatTime(now),
	); err != nil {
		return domain.Claim{}, err
	}
	for _, evidenceID := range input.EvidenceIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO claim_evidence(claim_id, evidence_id, linked_at) VALUES (?, ?, ?)`, id, evidenceID, formatTime(now)); err != nil {
			return domain.Claim{}, err
		}
	}
	if err := s.recordContradictionsTx(ctx, tx, id, input, normalized, now); err != nil {
		return domain.Claim{}, err
	}
	if err := s.appendMemoryEventTx(ctx, tx, "CLAIM_RECORDED", id, map[string]any{
		"subject_id": input.SubjectID,
		"predicate":  input.Predicate,
		"status":     input.Status,
	}, now); err != nil {
		return domain.Claim{}, err
	}
	return loadClaim(ctx, tx, id)
}

func (s *Service) recordContradictionsTx(ctx context.Context, tx *sql.Tx, newID domain.ID, input ClaimInput, normalized string, now time.Time) error {
	query := `
		SELECT claim_id FROM claims
		WHERE claim_id <> ? AND subject_id = ? AND predicate = ? AND normalized_statement <> ?
		  AND (valid_to IS NULL OR valid_to >= ?)`
	args := []any{newID, input.SubjectID, input.Predicate, normalized, formatTime(input.ValidFrom)}
	if !input.ValidTo.IsZero() {
		query += ` AND valid_from <= ?`
		args = append(args, formatTime(input.ValidTo))
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	var existing []domain.ID
	for rows.Next() {
		var id domain.ID
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		existing = append(existing, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range existing {
		left, right := id, newID
		if string(left) > string(right) {
			left, right = right, left
		}
		if err := insertRelationTx(ctx, tx, left, right, "CONTRADICTS", now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) putEvidenceTx(ctx context.Context, tx *sql.Tx, input EvidenceInput) error {
	ref := input.Ref
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO evidence_objects(evidence_id, content_hash, media_type, kind, size_bytes, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, ref.ID, ref.ContentHash, ref.MediaType, input.Kind, ref.SizeBytes, formatTime(ref.CreatedAt)); err != nil {
		return err
	}
	var hash, mediaType, kind, createdAt string
	var size int64
	if err := tx.QueryRowContext(ctx, `SELECT content_hash, media_type, kind, size_bytes, created_at FROM evidence_objects WHERE evidence_id = ?`, ref.ID).
		Scan(&hash, &mediaType, &kind, &size, &createdAt); err != nil {
		return err
	}
	if hash != ref.ContentHash || mediaType != ref.MediaType || kind != input.Kind || size != ref.SizeBytes || createdAt != formatTime(ref.CreatedAt) {
		return fmt.Errorf("evidence id %s already exists with different immutable metadata", ref.ID)
	}
	return nil
}

func (s *Service) validateEvidence(input EvidenceInput) (EvidenceInput, error) {
	input.Ref.ID = domain.ID(strings.TrimSpace(string(input.Ref.ID)))
	input.Ref.ContentHash = strings.TrimSpace(input.Ref.ContentHash)
	input.Ref.MediaType = strings.TrimSpace(input.Ref.MediaType)
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Ref.ID == "" || input.Ref.ContentHash == "" || input.Ref.MediaType == "" || input.Kind == "" {
		return EvidenceInput{}, errors.New("evidence id, content hash, media type, and kind are required")
	}
	if input.Ref.SizeBytes < 0 {
		return EvidenceInput{}, errors.New("evidence size must not be negative")
	}
	if input.Ref.CreatedAt.IsZero() {
		input.Ref.CreatedAt = s.clock.Now().UTC()
	} else {
		input.Ref.CreatedAt = input.Ref.CreatedAt.UTC()
	}
	return input, nil
}

func validateClaimInput(input ClaimInput) (ClaimInput, error) {
	input.ID = domain.ID(strings.TrimSpace(string(input.ID)))
	input.SubjectID = domain.ID(strings.TrimSpace(string(input.SubjectID)))
	input.Predicate = strings.TrimSpace(input.Predicate)
	input.Statement = strings.TrimSpace(input.Statement)
	input.SourceID = domain.ID(strings.TrimSpace(string(input.SourceID)))
	if input.SubjectID == "" || input.Predicate == "" || input.Statement == "" {
		return ClaimInput{}, errors.New("claim subject, predicate, and statement are required")
	}
	if !validClaimStatus(input.Status) {
		return ClaimInput{}, fmt.Errorf("invalid claim status %q", input.Status)
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return ClaimInput{}, errors.New("claim confidence must be between 0 and 1")
	}
	if input.ValidFrom.IsZero() {
		return ClaimInput{}, errors.New("claim valid_from is required")
	}
	input.ValidFrom = input.ValidFrom.UTC()
	if !input.ValidTo.IsZero() {
		input.ValidTo = input.ValidTo.UTC()
		if input.ValidTo.Before(input.ValidFrom) {
			return ClaimInput{}, errors.New("claim valid_to cannot precede valid_from")
		}
	}
	if len(input.EvidenceIDs) == 0 {
		return ClaimInput{}, errors.New("claim must cite at least one evidence object")
	}
	seen := map[domain.ID]struct{}{}
	clean := make([]domain.ID, 0, len(input.EvidenceIDs))
	for _, id := range input.EvidenceIDs {
		id = domain.ID(strings.TrimSpace(string(id)))
		if id == "" {
			return ClaimInput{}, errors.New("claim evidence id must not be empty")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	input.EvidenceIDs = clean
	return input, nil
}

func validClaimStatus(status domain.ClaimStatus) bool {
	switch status {
	case domain.ClaimSpeculation, domain.ClaimHypothesis, domain.ClaimSupported, domain.ClaimVerified:
		return true
	default:
		return false
	}
}

func insertRelationTx(ctx context.Context, tx *sql.Tx, left, right domain.ID, kind string, at time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO claim_relations(relation_id, left_claim_id, right_claim_id, kind, created_at)
		VALUES (?, ?, ?, ?, ?)`, domain.NewID("claimrel"), left, right, kind, formatTime(at.UTC()))
	return err
}

func (s *Service) appendMemoryEventTx(ctx context.Context, tx *sql.Tx, kind string, subjectID domain.ID, payload any, at time.Time) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO memory_events(event_id, kind, subject_id, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		domain.NewID("memoryevent"), kind, subjectID, string(data), formatTime(at.UTC()))
	return err
}

func loadClaim(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id domain.ID) (domain.Claim, error) {
	var claim domain.Claim
	var validFrom, createdAt string
	var validTo sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT claim_id, statement, status, confidence, valid_from, valid_to, created_at
		FROM claims WHERE claim_id = ?`, id).
		Scan(&claim.ID, &claim.Statement, &claim.Status, &claim.Confidence, &validFrom, &validTo, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Claim{}, fmt.Errorf("claim %s not found", id)
	}
	if err != nil {
		return domain.Claim{}, err
	}
	claim.ValidFrom, err = parseTime(validFrom)
	if err != nil {
		return domain.Claim{}, err
	}
	if validTo.Valid {
		claim.ValidTo, err = parseTime(validTo.String)
		if err != nil {
			return domain.Claim{}, err
		}
	}
	claim.CreatedAt, err = parseTime(createdAt)
	return claim, err
}

func scanClaims(rows *sql.Rows) ([]domain.Claim, error) {
	var claims []domain.Claim
	for rows.Next() {
		var claim domain.Claim
		var validFrom, createdAt string
		var validTo sql.NullString
		if err := rows.Scan(&claim.ID, &claim.Statement, &claim.Status, &claim.Confidence, &validFrom, &validTo, &createdAt); err != nil {
			return nil, err
		}
		var err error
		claim.ValidFrom, err = parseTime(validFrom)
		if err != nil {
			return nil, err
		}
		if validTo.Valid {
			claim.ValidTo, err = parseTime(validTo.String)
			if err != nil {
				return nil, err
			}
		}
		claim.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	return claims, rows.Err()
}

func normalizeStatement(statement string) string {
	return strings.ToLower(strings.Join(strings.Fields(statement), " "))
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t.UTC())
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}

func (s *Service) configured() error {
	if s == nil || s.store == nil || s.store.DB() == nil || s.clock == nil {
		return errors.New("memory service is not configured")
	}
	return nil
}
