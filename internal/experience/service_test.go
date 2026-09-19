package experience

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/approvals"
	"github.com/SofiaFlux/meeseek-collective/internal/audit"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
)

type expHarness struct {
	ctx context.Context
	svc *Service
	approvals *approvals.Service
	clock *testutil.Clock
	store interface{ DB() interface{} }
}

func newExpHarness(t *testing.T) (*Service,*approvals.Service,*testutil.Clock) {
	t.Helper()
	store:=testutil.OpenStore(t)
	clk:=testutil.NewClock(time.Date(2026,9,19,12,0,0,0,time.UTC))
	ap:=approvals.New(store,clk)
	svc:=New(store,clk,ap,audit.New(store,clk),"owner-1")
	return svc,ap,clk
}

func TestGrantRequiresExactOwnerApprovalAndHasNoImplicitAuthority(t *testing.T) {
	svc,ap,clk:=newExpHarness(t)
	ctx:=context.Background()
	req,err:=svc.RequestGrant(ctx,GrantInput{
		Kind:AdaptationExecutorPreference,ScopeKey:"repo.review",
		AllowedExecutors:[]string{"codex","claude"},MinVerifiedSamples:3,
		MaxAcceptanceRegressionBps:500,MaxCostRegressionBps:1000,ExpiresAt:clk.Now().Add(24*time.Hour),
	},"requester-1")
	if err!=nil{t.Fatal(err)}
	var count int
	if err:=svc.store.DB().QueryRowContext(ctx,"SELECT count(*) FROM adaptation_grants").Scan(&count);err!=nil{t.Fatal(err)}
	if count!=0{t.Fatalf("grant exists before approval: %d",count)}
	if _,err:=svc.ActivateGrant(ctx,req.ID);err==nil{t.Fatal("grant activated before Owner approval")}
	if err:=ap.Approve(ctx,req.ApprovalID,"owner-1","wrong-digest");err==nil{t.Fatal("wrong digest approval succeeded")}
	if err:=ap.Approve(ctx,req.ApprovalID,"owner-1",req.Digest);err!=nil{t.Fatal(err)}
	grant,err:=svc.ActivateGrant(ctx,req.ID);if err!=nil{t.Fatal(err)}
	if grant.OwnerPrincipalID!="owner-1"||grant.Kind!=AdaptationExecutorPreference{t.Fatalf("grant=%+v",grant)}
	approval,err:=ap.Get(ctx,req.ApprovalID);if err!=nil{t.Fatal(err)}
	if approval.State!=domain.ApprovalConsumed{t.Fatalf("approval state=%s",approval.State)}
}

func TestGrantRejectsTrustedControlKindsAndProposalRequiresEvidence(t *testing.T) {
	svc,ap,clk:=newExpHarness(t)
	ctx:=context.Background()
	if _,err:=svc.RequestGrant(ctx,GrantInput{
		Kind:"POLICY_OVERRIDE",ScopeKey:"repo",AllowedExecutors:[]string{"codex"},MinVerifiedSamples:1,ExpiresAt:clk.Now().Add(time.Hour),
	},"requester");err==nil{t.Fatal("policy override grant accepted")}
	req,err:=svc.RequestGrant(ctx,GrantInput{
		Kind:AdaptationExecutorPreference,ScopeKey:"repo.review",AllowedExecutors:[]string{"codex"},MinVerifiedSamples:1,ExpiresAt:clk.Now().Add(time.Hour),
	},"requester");if err!=nil{t.Fatal(err)}
	if err:=ap.Approve(ctx,req.ApprovalID,"owner-1",req.Digest);err!=nil{t.Fatal(err)}
	grant,err:=svc.ActivateGrant(ctx,req.ID);if err!=nil{t.Fatal(err)}
	if _,err:=svc.Propose(ctx,ProposalInput{
		GrantID:grant.ID,GenericTaskClass:domain.GenericTaskReview,ScopeKey:"repo.review",
		PreferredExecutor:"codex",EvidenceObservationIDs:[]domain.ID{"missing-observation"},
	});err==nil{t.Fatal("proposal accepted missing evidence")}
	if _,err:=svc.Propose(ctx,ProposalInput{
		GrantID:grant.ID,GenericTaskClass:domain.GenericTaskReview,ScopeKey:"repo.review",
		PreferredExecutor:"not-allowed",EvidenceObservationIDs:[]domain.ID{"missing"},
	});err==nil{t.Fatal("proposal accepted executor outside grant")}
}
