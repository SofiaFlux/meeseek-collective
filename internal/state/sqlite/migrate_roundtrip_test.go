package sqlite

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestFieldFeedbackAndRunManifestMigrationsRoundTripWithExistingTask(t *testing.T) {
	ctx:=context.Background()
	path:=filepath.Join(t.TempDir(),"roundtrip.db")
	store,err:=Open(ctx,path);if err!=nil{t.Fatal(err)}
	defer store.DB().Close()
	db:=store.DB()

	if _,err:=db.ExecContext(ctx,`
		INSERT INTO resource_envelopes(envelope_id,hard_limit,created_at)
		VALUES ('env-roundtrip',100,'2026-09-21T18:00:00Z')`);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`
		INSERT INTO tasks(task_id,purpose_kind,purpose_id,task_class,state,current_attempt_id,current_fence,
			acceptance_criteria_json,required_capabilities_json,required_enforcement,authority_ceiling_json,
			resource_envelope_id,priority,created_at,updated_at)
		VALUES ('task-roundtrip','OWNER_DIRECTIVE','owner-roundtrip','repo.review','EXECUTING','attempt-roundtrip',1,
			'["ok"]','[]','ENFORCED','[]','env-roundtrip',0,'2026-09-21T18:00:00Z','2026-09-21T18:00:00Z')`);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`
		INSERT INTO attempts(attempt_id,task_id,state,fence_generation,lease_state,lease_expires_at,started_at,executor_kind)
		VALUES ('attempt-roundtrip','task-roundtrip','RUNNING',1,'ACTIVE','2026-09-21T19:00:00Z','2026-09-21T18:00:00Z','codex')`);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,`
		INSERT INTO attempt_run_manifests(attempt_id,task_id,manifest_hash,manifest_json,created_at)
		VALUES ('attempt-roundtrip','task-roundtrip','hash-roundtrip','{"schema_version":1}','2026-09-21T18:00:00Z')`);err!=nil{t.Fatal(err)}

	migrations,err:=fs.Sub(migrationFiles,"migrations");if err!=nil{t.Fatal(err)}
	provider,err:=goose.NewProvider(goose.DialectSQLite3,db,migrations,goose.WithTableName("schema_migrations"))
	if err!=nil{t.Fatal(err)}

	if _,err:=provider.DownTo(ctx,9);err!=nil{t.Fatalf("down to v9: %v",err)}
	var count int
	if err:=db.QueryRowContext(ctx,`SELECT count(*) FROM pragma_table_info('tasks') WHERE name='task_class'`).Scan(&count);err!=nil{t.Fatal(err)}
	if count!=0{t.Fatalf("task_class survived down migration: %d",count)}
	if err:=db.QueryRowContext(ctx,`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='attempt_run_manifests'`).Scan(&count);err!=nil{t.Fatal(err)}
	if count!=0{t.Fatalf("attempt_run_manifests survived down migration: %d",count)}
	var taskState string
	if err:=db.QueryRowContext(ctx,"SELECT state FROM tasks WHERE task_id='task-roundtrip'").Scan(&taskState);err!=nil{t.Fatal(err)}
	if taskState!="EXECUTING"{t.Fatalf("base Task lost during down migration: state=%s",taskState)}

	if _,err:=provider.UpTo(ctx,12);err!=nil{t.Fatalf("up to v12: %v",err)}
	var taskClass string
	if err:=db.QueryRowContext(ctx,"SELECT task_class FROM tasks WHERE task_id='task-roundtrip'").Scan(&taskClass);err!=nil{t.Fatal(err)}
	if taskClass!=""{t.Fatalf("re-added task_class=%q, want safe empty default",taskClass)}
	if err:=db.QueryRowContext(ctx,`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='attempt_run_manifests'`).Scan(&count);err!=nil{t.Fatal(err)}
	if count!=1{t.Fatalf("attempt_run_manifests table count after re-up=%d",count)}
	for _,trigger:=range []string{"approval_requests_binding_immutable","experience_rules_no_update","attempt_run_manifests_no_update"}{
		var name string
		if err:=db.QueryRowContext(ctx,`SELECT name FROM sqlite_master WHERE type='trigger' AND name=?`,trigger).Scan(&name);err!=nil{
			t.Fatalf("trigger %s missing after re-up: %v",trigger,err)
		}
	}
}
