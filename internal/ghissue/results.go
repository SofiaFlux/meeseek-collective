package ghissue

// ExcludedIssue records one issue that did not become a case, with its reason
// token. It is defined once here and reused by observe.go.
type ExcludedIssue struct {
	Issue  Issue
	Reason string
}
