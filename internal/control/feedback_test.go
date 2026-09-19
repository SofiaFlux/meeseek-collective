package control

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/fieldfeedback"
)

type fakeFeedbackControl struct {
	candidate domain.FeedbackCandidate
	artifact  domain.SanitizedFeedback
	task      domain.Task
}

func (f *fakeFeedbackControl) Candidates(context.Context, domain.FeedbackCandidateState) ([]domain.FeedbackCandidate,error) {
	return []domain.FeedbackCandidate{f.candidate},nil
}
func (f *fakeFeedbackControl) Candidate(context.Context, domain.ID) (domain.FeedbackCandidate,error) {
	return f.candidate,nil
}
func (f *fakeFeedbackControl) LatestSanitizedFeedbackForCandidate(context.Context, domain.ID) (domain.SanitizedFeedback,bool,error) {
	return f.artifact,f.artifact.ID!="",nil
}
func (f *fakeFeedbackControl) RequestEmit(context.Context, domain.ID) (domain.Task,error) {
	return f.task,nil
}

type fakeFieldObserver struct {
	observation domain.FieldObservation
}

func (f *fakeFieldObserver) Record(context.Context, fieldfeedback.ObservationInput) (domain.FieldObservation,error) {
	return f.observation,nil
}
func (f *fakeFieldObserver) Scan(context.Context) ([]domain.FieldObservation,error) {
	return []domain.FieldObservation{f.observation},nil
}

func TestFeedbackControlSeparatesLocalCandidateFromExportArtifact(t *testing.T) {
	server,_,_,_,_,_,_:=newTestServer(t)
	server.deps.Feedback=&fakeFeedbackControl{
		candidate:domain.FeedbackCandidate{
			ID:"candidate-1",State:domain.FeedbackStateSanitized,GenericTaskClass:domain.GenericTaskDebugging,
			Category:"RECOVERY_FRICTION",ExpectedBehavior:"generic expected",
			ObservedBehavior:"LOCAL SECRET client path /home/acme/repo",Enforcement:domain.EnforcementEnforced,
		},
		artifact:domain.SanitizedFeedback{
			ID:"sanitized-1",CandidateID:"candidate-1",SchemaVersion:1,
			ContentJSON:`{"category":"RECOVERY_FRICTION","observed_behavior":"generic safe"}`,
			ContentHash:"hash",CorrelationKey:"corr",Fingerprint:"fp",
		},
		task:domain.Task{ID:"emit-task-1"},
	}
	httpServer:=httptest.NewServer(server.Handler());defer httpServer.Close()

	response:=doRequest(t,http.MethodGet,httpServer.URL+"/feedback/candidate-1","control-secret",nil)
	if response.StatusCode!=http.StatusOK{t.Fatalf("inspect status=%d",response.StatusCode)}
	var dto FeedbackInspectDTO
	decodeJSON(t,response,&dto)
	if dto.LocalCandidate==nil||dto.ExportArtifact==nil{t.Fatalf("inspect=%+v",dto)}
	if !strings.Contains(dto.LocalCandidate.ObservedBehavior,"LOCAL SECRET"){
		t.Fatalf("local section lost local candidate: %+v",dto.LocalCandidate)
	}
	if strings.Contains(dto.ExportArtifact.ContentJSON,"LOCAL SECRET")||strings.Contains(dto.ExportArtifact.ContentJSON,"/home/acme"){
		t.Fatalf("export artifact leaked local content: %+v",dto.ExportArtifact)
	}

	response=doRequest(t,http.MethodPost,httpServer.URL+"/feedback/candidate-1/emit","control-secret",nil)
	if response.StatusCode!=http.StatusAccepted{t.Fatalf("emit status=%d",response.StatusCode)}
	var emitted FeedbackEmitDTO
	decodeJSON(t,response,&emitted)
	if emitted.Status!="GOVERNED_WORK_SCHEDULED"||emitted.TaskID!="emit-task-1"{t.Fatalf("emit=%+v",emitted)}
}

func TestFeedbackObserveAndScanNeverReturnLocalSummary(t *testing.T) {
	server,_,_,_,_,_,_:=newTestServer(t)
	server.deps.FieldObserver=&fakeFieldObserver{observation:domain.FieldObservation{
		ID:"obs-1",TaskID:"task-1",Category:"RECOVERY_FRICTION",SourceKind:"OPERATOR",
		SummaryLocal:"TOP SECRET local summary",
	}}
	httpServer:=httptest.NewServer(server.Handler());defer httpServer.Close()

	response:=doRequest(t,http.MethodPost,httpServer.URL+"/feedback/observations","control-secret",
		bytes.NewBufferString(`{"task_id":"task-1","category":"RECOVERY_FRICTION","summary_local":"TOP SECRET local summary"}`))
	if response.StatusCode!=http.StatusCreated{t.Fatalf("observe status=%d",response.StatusCode)}
	var observed FeedbackObservationDTO
	decodeJSON(t,response,&observed)
	if observed.ID!="obs-1"{t.Fatalf("observe=%+v",observed)}

	response=doRequest(t,http.MethodPost,httpServer.URL+"/feedback/scan","control-secret",nil)
	if response.StatusCode!=http.StatusOK{t.Fatalf("scan status=%d",response.StatusCode)}
	raw:=readResponse(t,response)
	if strings.Contains(raw,"TOP SECRET")||strings.Contains(raw,"summary"){
		t.Fatalf("scan leaked local summary: %s",raw)
	}
}

func readResponse(t *testing.T,response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	var buf bytes.Buffer
	if _,err:=buf.ReadFrom(response.Body);err!=nil{t.Fatal(err)}
	return buf.String()
}
