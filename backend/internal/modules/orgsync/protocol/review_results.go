package protocol

import "bytes"

type ReviewSelector struct {
	redacted
	OperationID, PayloadDigest string
}

type ReviewExpectation struct {
	redacted
	Selector     ReviewSelector
	ReviewItemID string
}

type ReviewResultsRequestSnapshot struct {
	redacted
	RequestID, MembershipID, EnrollmentID string
	Proof                                 DeliveryProof
	Selectors                             []ReviewSelector
}

// ReviewResultsRequest is strict transport data, not authenticated source proof.
type ReviewResultsRequest struct {
	redacted
	value     ReviewResultsRequestSnapshot
	canonical []byte
}

func NewReviewResultsRequest(value ReviewResultsRequestSnapshot) (ReviewResultsRequest, error) {
	if !uuid(value.RequestID) || !uuid(value.MembershipID) || !uuid(value.EnrollmentID) || !uuid(value.Proof.FamilyID) || len(value.Proof.AccessToken) < 1 || len(value.Proof.AccessToken) > 512 {
		return ReviewResultsRequest{}, ErrInvalid
	}
	for _, b := range []byte(value.Proof.AccessToken) {
		if b < 0x21 || b > 0x7e {
			return ReviewResultsRequest{}, ErrInvalid
		}
	}
	if len(value.Selectors) > MaxReviewResultSelectors {
		return ReviewResultsRequest{}, ErrLimit
	}
	copy := ReviewResultsRequest{value: value}.Snapshot()
	selectors := &node{kind: 'a'}
	seen := make(map[string]bool, len(copy.Selectors))
	for _, selector := range copy.Selectors {
		if !uuid(selector.OperationID) || !digestText(selector.PayloadDigest) || seen[selector.OperationID] {
			return ReviewResultsRequest{}, ErrInvalid
		}
		seen[selector.OperationID] = true
		selectors.list = append(selectors.list, reviewSelectorNode(selector))
	}
	root := &node{kind: 'o', fields: map[string]*node{"schema_version": reviewInteger(1), "request_id": reviewText(copy.RequestID), "family_id": reviewText(copy.Proof.FamilyID), "access_token": reviewText(copy.Proof.AccessToken), "membership_id": reviewText(copy.MembershipID), "enrollment_id": reviewText(copy.EnrollmentID), "selectors": selectors}}
	raw := canonical(root)
	if len(raw) > MaxReviewResultsRequestBytes {
		return ReviewResultsRequest{}, ErrLimit
	}
	return ReviewResultsRequest{value: copy, canonical: raw}, nil
}
func ParseReviewResultsRequest(raw []byte) (ReviewResultsRequest, error) {
	if len(raw) > MaxReviewResultsRequestBytes {
		return ReviewResultsRequest{}, ErrLimit
	}
	root, err := parseJSON(raw, MaxTransportDepth)
	if err != nil {
		return ReviewResultsRequest{}, err
	}
	if !shape(root, "schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "selectors") {
		return ReviewResultsRequest{}, ErrInvalid
	}
	version, ok := integer(root.fields["schema_version"])
	if !ok || version != 1 {
		return ReviewResultsRequest{}, ErrInvalid
	}
	v := ReviewResultsRequestSnapshot{}
	for key, destination := range map[string]*string{"request_id": &v.RequestID, "family_id": &v.Proof.FamilyID, "access_token": &v.Proof.AccessToken, "membership_id": &v.MembershipID, "enrollment_id": &v.EnrollmentID} {
		*destination, ok = stringValue(root.fields[key])
		if !ok {
			return ReviewResultsRequest{}, ErrInvalid
		}
	}
	selectors := root.fields["selectors"]
	if selectors.kind != 'a' {
		return ReviewResultsRequest{}, ErrInvalid
	}
	if len(selectors.list) > MaxReviewResultSelectors {
		return ReviewResultsRequest{}, ErrLimit
	}
	for _, item := range selectors.list {
		s, e := parseReviewSelector(item)
		if e != nil {
			return ReviewResultsRequest{}, e
		}
		v.Selectors = append(v.Selectors, s)
	}
	return NewReviewResultsRequest(v)
}
func (request ReviewResultsRequest) Snapshot() ReviewResultsRequestSnapshot {
	value := request.value
	value.Selectors = append([]ReviewSelector(nil), value.Selectors...)
	return value
}
func (request ReviewResultsRequest) CanonicalBytes() []byte {
	return append([]byte(nil), request.canonical...)
}
func (request ReviewResultsRequest) ValidateSource(source SourceBinding) error {
	if len(request.canonical) == 0 {
		return ErrInvalid
	}
	if !source.binding.valid() || !uuid(source.member) || !uuid(source.enrollment) || request.value.MembershipID != source.member || request.value.EnrollmentID != source.enrollment {
		return ErrBinding
	}
	return nil
}

type ReviewResult struct {
	redacted
	Selector   ReviewSelector
	Resolution *ReviewResolution
}

type ReviewResultsResponse struct {
	redacted
	results   []ReviewResult
	canonical []byte
}

func NewReviewResultsResponse(request ReviewResultsRequest, source SourceBinding, results []ReviewResult) (ReviewResultsResponse, error) {
	if err := request.ValidateSource(source); err != nil {
		return ReviewResultsResponse{}, err
	}
	if len(results) != len(request.value.Selectors) {
		return ReviewResultsResponse{}, ErrBinding
	}
	values := make([]ReviewResult, 0, len(results))
	list := &node{kind: 'a'}
	for i, result := range results {
		if err := reviewSelectorMatches(result.Selector, request.value.Selectors[i]); err != nil {
			return ReviewResultsResponse{}, err
		}
		item := reviewSelectorNode(result.Selector)
		item.fields["resolution"] = &node{kind: 'n', value: "null"}
		if result.Resolution != nil {
			copy, err := NewReviewResolution(result.Resolution.Snapshot())
			if err != nil {
				return ReviewResultsResponse{}, err
			}
			if !bytes.Equal(copy.canonical, result.Resolution.canonical) {
				return ReviewResultsResponse{}, ErrInvalid
			}
			if err := reviewSelectorMatches(ReviewSelector{OperationID: copy.value.OperationID, PayloadDigest: copy.value.PayloadDigest}, result.Selector); err != nil {
				return ReviewResultsResponse{}, err
			}
			item.fields["resolution"], err = parseJSON(copy.canonical, MaxTransportDepth)
			if err != nil {
				return ReviewResultsResponse{}, err
			}
			result.Resolution = &copy
		}
		list.list = append(list.list, item)
		values = append(values, result)
	}
	root := &node{kind: 'o', fields: map[string]*node{"schema_version": reviewInteger(1), "request_id": reviewText(request.value.RequestID), "owner_account_id": reviewText(source.binding.owner), "api_origin": reviewText(source.binding.origin), "org_id": reviewText(source.binding.organization), "membership_id": reviewText(source.member), "enrollment_id": reviewText(source.enrollment), "results": list}}
	raw := canonical(root)
	if len(raw) > MaxReviewResultsResponseBytes {
		return ReviewResultsResponse{}, ErrLimit
	}
	return ReviewResultsResponse{results: values, canonical: raw}, nil
}
func ParseReviewResultsResponse(raw []byte, request ReviewResultsRequest, source SourceBinding, expected []ReviewExpectation) (ReviewResultsResponse, error) {
	if err := request.ValidateSource(source); err != nil {
		return ReviewResultsResponse{}, err
	}
	if len(raw) > MaxReviewResultsResponseBytes {
		return ReviewResultsResponse{}, ErrLimit
	}
	if len(expected) != len(request.value.Selectors) {
		return ReviewResultsResponse{}, ErrBinding
	}
	for i, v := range expected {
		if err := reviewSelectorMatches(v.Selector, request.value.Selectors[i]); err != nil {
			return ReviewResultsResponse{}, err
		}
		if !uuid(v.ReviewItemID) {
			return ReviewResultsResponse{}, ErrBinding
		}
	}
	root, err := parseJSON(raw, MaxTransportDepth)
	if err != nil {
		return ReviewResultsResponse{}, err
	}
	if !shape(root, "schema_version", "request_id", "owner_account_id", "api_origin", "org_id", "membership_id", "enrollment_id", "results") {
		return ReviewResultsResponse{}, ErrInvalid
	}
	version, ok := integer(root.fields["schema_version"])
	if !ok || version != 1 {
		return ReviewResultsResponse{}, ErrInvalid
	}
	for key, want := range map[string]string{"request_id": request.value.RequestID, "owner_account_id": source.binding.owner, "api_origin": source.binding.origin, "org_id": source.binding.organization, "membership_id": source.member, "enrollment_id": source.enrollment} {
		got, ok := stringValue(root.fields[key])
		if !ok {
			return ReviewResultsResponse{}, ErrInvalid
		}
		if got != want {
			return ReviewResultsResponse{}, ErrBinding
		}
	}
	list := root.fields["results"]
	if list.kind != 'a' {
		return ReviewResultsResponse{}, ErrInvalid
	}
	if len(list.list) != len(expected) {
		return ReviewResultsResponse{}, ErrBinding
	}
	results := make([]ReviewResult, 0, len(expected))
	for i, item := range list.list {
		if !shape(item, "operation_id", "payload_digest", "resolution") {
			return ReviewResultsResponse{}, ErrInvalid
		}
		selector, err := parseReviewSelectorFields(item)
		if err != nil {
			return ReviewResultsResponse{}, err
		}
		if err := reviewSelectorMatches(selector, expected[i].Selector); err != nil {
			return ReviewResultsResponse{}, err
		}
		result := ReviewResult{Selector: selector}
		resolution := item.fields["resolution"]
		if resolution.kind != 'n' {
			if resolution.end-resolution.start > MaxReviewResolutionBytes {
				return ReviewResultsResponse{}, ErrLimit
			}
			value, err := reviewResolutionNode(resolution)
			if err != nil {
				return ReviewResultsResponse{}, err
			}
			if err := reviewSelectorMatches(ReviewSelector{OperationID: value.value.OperationID, PayloadDigest: value.value.PayloadDigest}, selector); err != nil {
				return ReviewResultsResponse{}, err
			}
			if value.value.ReviewItemID != expected[i].ReviewItemID {
				return ReviewResultsResponse{}, ErrBinding
			}
			result.Resolution = &value
		}
		results = append(results, result)
	}
	return NewReviewResultsResponse(request, source, results)
}
func (response ReviewResultsResponse) CanonicalBytes() []byte {
	return append([]byte(nil), response.canonical...)
}
func (response ReviewResultsResponse) Results() []ReviewResult {
	values := append([]ReviewResult(nil), response.results...)
	for i := range values {
		if values[i].Resolution != nil {
			copy := *values[i].Resolution
			values[i].Resolution = &copy
		}
	}
	return values
}

func reviewSelectorNode(value ReviewSelector) *node {
	return &node{kind: 'o', fields: map[string]*node{"operation_id": reviewText(value.OperationID), "payload_digest": reviewText(value.PayloadDigest)}}
}
func parseReviewSelector(value *node) (ReviewSelector, error) {
	if !shape(value, "operation_id", "payload_digest") {
		return ReviewSelector{}, ErrInvalid
	}
	return parseReviewSelectorFields(value)
}
func parseReviewSelectorFields(value *node) (ReviewSelector, error) {
	id, ok := stringValue(value.fields["operation_id"])
	if !ok || !uuid(id) {
		return ReviewSelector{}, ErrInvalid
	}
	digest, ok := stringValue(value.fields["payload_digest"])
	if !ok || !digestText(digest) {
		return ReviewSelector{}, ErrInvalid
	}
	return ReviewSelector{OperationID: id, PayloadDigest: digest}, nil
}
func reviewSelectorMatches(value, want ReviewSelector) error {
	if value.OperationID != want.OperationID {
		return ErrBinding
	}
	if value.PayloadDigest != want.PayloadDigest {
		return ErrDigest
	}
	return nil
}
