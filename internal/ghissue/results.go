package ghissue

// ExcludedIssue records one issue that did not become a case, with its reason
// token. It is defined once here and reused by observe.go. Detail carries the
// underlying cause of a reason whose token alone does not explain it, such as
// ensure-failed; it is empty when the token is the whole story.
type ExcludedIssue struct {
	Issue  Issue
	Reason string
	Detail string
}
