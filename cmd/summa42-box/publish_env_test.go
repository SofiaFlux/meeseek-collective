package main

import (
	"reflect"
	"testing"

	"github.com/SofiaFlux/summa42/internal/adoreview"
	"github.com/SofiaFlux/summa42/internal/domain"
)

func clearPublishEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SUMMA42_ADO_MCP_COMMAND",
		"SUMMA42_ADO_ORGANIZATION",
		"SUMMA42_PUBLISH_MODE",
		"SUMMA42_PUBLISH_APPROVERS",
		"SUMMA42_PUBLISH_RISK_COMMENT",
		"SUMMA42_PUBLISH_RISK_APPROVE",
	} {
		t.Setenv(key, "")
	}
}

func TestPublishEnvAbsentRegistersNothing(t *testing.T) {
	clearPublishEnv(t)
	settings, providers, err := buildPublishFromEnv(nil)
	if err != nil || settings.mode != "" || len(settings.approvers) != 0 || settings.riskComment != "" || settings.riskApprove != "" || providers != nil {
		t.Fatalf("got %#v, %v, %v; want nil registration", settings, providers, err)
	}
}

func TestPublishEnvRejectsBadMode(t *testing.T) {
	clearPublishEnv(t)
	t.Setenv("SUMMA42_PUBLISH_MODE", "everything")
	if _, _, err := buildPublishFromEnv(nil); err == nil {
		t.Fatal("accepted bad mode")
	}
}

func TestPublishEnvRequiresApproveRiskInAllMode(t *testing.T) {
	clearPublishEnv(t)
	t.Setenv("SUMMA42_PUBLISH_MODE", "all")
	t.Setenv("SUMMA42_PUBLISH_RISK_APPROVE", "")
	if _, _, err := buildPublishFromEnv(nil); err == nil {
		t.Fatal("accepted all mode without approve risk")
	}
}

func TestPublishEnvBuildsProvidersAndSettings(t *testing.T) {
	clearPublishEnv(t)
	t.Setenv("SUMMA42_ADO_MCP_COMMAND", "/bin/true")
	t.Setenv("SUMMA42_ADO_ORGANIZATION", "Contoso")
	t.Setenv("SUMMA42_PUBLISH_MODE", "comments")
	t.Setenv("SUMMA42_PUBLISH_APPROVERS", "owner-1, owner-2")
	t.Setenv("SUMMA42_PUBLISH_RISK_COMMENT", "MEDIUM")
	t.Setenv("SUMMA42_PUBLISH_RISK_APPROVE", "HIGH")
	provider, err := buildADOProviderFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	settings, providers, err := buildPublishFromEnv(provider)
	if err != nil {
		t.Fatal(err)
	}
	if settings.mode != adoreview.PublishComments || !reflect.DeepEqual(settings.approvers, []domain.ID{"owner-1", "owner-2"}) || settings.riskComment != "MEDIUM" || settings.riskApprove != "HIGH" {
		t.Fatalf("settings = %#v", settings)
	}
	if len(providers) != 2 || providers[0].Name() != "ado-pr-comment" || providers[1].Name() != "ado-pr-vote" {
		t.Fatalf("providers = %v", providers)
	}
}
