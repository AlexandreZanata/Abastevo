package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/url"
	"time"
)

// runCommunity is restricted to the fixed owned staging origin and a synthetic
// station provisioned by the operator. Keys, upload capabilities and photos are
// never printed. The operator retires only these synthetic facts after proof.
func (p *probe) runCommunity(ctx context.Context, station string) error {
	if len(station) != 36 {
		return errors.New("synthetic station UUID required")
	}
	a, err := p.enroll(ctx)
	if err != nil {
		return err
	}
	b, err := p.enroll(ctx)
	if err != nil {
		return err
	}
	p.checks = append(p.checks, "two-synthetic-owners-enrolled")
	call := func(o *owner, method, path string, body any, status int) (map[string]any, error) {
		var raw []byte
		if body != nil {
			raw = payload(body)
		}
		r, err := p.signed(ctx, o, method, path, raw, false)
		if err != nil {
			return nil, err
		}
		return decode(r, status, "", 0)
	}
	public := func(path string) (map[string]any, error) {
		r, err := p.send(request(ctx, "GET", path, nil))
		if err != nil {
			return nil, err
		}
		if r.status != 200 {
			return nil, fmt.Errorf("public HTTP %d", r.status)
		}
		var v map[string]any
		err = json.Unmarshal(r.body, &v)
		return v, err
	}
	capture, err := call(a, "POST", "/v1/photo-captures", map[string]any{"client_capture_id": "staging-community-camera", "station_id": station}, 201)
	if err != nil {
		return err
	}
	if capture["policy_version"] != "photo-capture-community-v2" {
		return errors.New("normal no-GPS camera policy missing")
	}
	captureID, ok := capture["capture_id"].(string)
	if !ok {
		return errors.New("capture missing")
	}
	var photo bytes.Buffer
	if err := jpeg.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 6, 6)), &jpeg.Options{Quality: 90}); err != nil {
		return err
	}
	sum := sha256.Sum256(photo.Bytes())
	captured := time.Now().UTC().Format(time.RFC3339Nano)
	up, err := call(a, "POST", "/v1/uploads", map[string]any{"client_submission_id": "staging-community-upload", "content_type": "image/jpeg", "size_bytes": photo.Len(), "sha256": fmt.Sprintf("%x", sum), "photo_capture_id": captureID, "station_id": station, "captured_at": captured}, 201)
	if err != nil {
		return err
	}
	capability, ok := up["url"].(string)
	if !ok {
		return errors.New("upload capability missing")
	}
	u, err := url.Parse(capability)
	if err != nil || u.Scheme != "https" || u.Host != "teste.abastevo.com.br" || u.User != nil {
		return errors.New("unexpected upload origin")
	}
	put, err := http.NewRequestWithContext(ctx, "PUT", capability, bytes.NewReader(photo.Bytes()))
	if err != nil {
		return err
	}
	put.Header.Set("Content-Type", "image/jpeg")
	uploaded, err := p.send(put)
	if err != nil || uploaded.status != 204 {
		return fmt.Errorf("synthetic upload status %d", uploaded.status)
	}
	uploadID, ok := up["upload_id"].(string)
	if !ok {
		return errors.New("upload id missing")
	}
	if _, err := call(b, "GET", "/v1/uploads/"+uploadID, nil, 404); err != nil {
		return fmt.Errorf("foreign media privacy: %w", err)
	}
	if _, err := call(a, "POST", "/v1/uploads/"+uploadID+"/complete", map[string]any{}, 202); err != nil {
		return err
	}
	evidence := ""
	for attempt := 0; attempt < 6; attempt++ {
		v, err := call(a, "GET", "/v1/uploads/"+uploadID, nil, 200)
		if err != nil {
			return err
		}
		evidence, _ = v["evidence_id"].(string)
		if evidence != "" {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if evidence == "" {
		return errors.New("worker evidence deadline exceeded")
	}
	p.checks = append(p.checks, "normal-camera-no-GPS-private-upload-worker-ready")
	for _, product := range []string{"GASOLINE_REGULAR", "ETHANOL"} {
		row := map[string]any{"client_submission_id": "staging-community-" + product, "station_id": station, "fuel_product": product, "price": map[string]any{"amount_milli_brl": 5899, "currency": "BRL", "unit": "L"}, "condition": map[string]any{"kind": "STANDARD"}, "evidence_id": evidence, "photo_capture_id": captureID, "claimed_captured_at": captured}
		if _, err := call(a, "POST", "/v1/observations", row, 201); err != nil {
			return err
		}
	}
	var target map[string]any
	for attempt := 0; attempt < 6; attempt++ {
		feed, err := public("/v1/community/feed?state=MT&municipality_code=5107925&fuel_product=ETHANOL&limit=50")
		if err != nil {
			return err
		}
		items, _ := feed["items"].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if item["station_id"] == station {
				target, _ = item["contribution"].(map[string]any)
			}
		}
		if target != nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if target == nil {
		return errors.New("worker projection absent from public feed")
	}
	observation, ok := target["observation_id"].(string)
	if !ok {
		return errors.New("public contribution id missing")
	}
	sequence, ok := target["decision_sequence"].(float64)
	if !ok || sequence < 1 {
		return errors.New("public contribution sequence missing")
	}
	prices, err := public("/v1/stations/" + station + "/prices?fuel_product=ETHANOL")
	if err != nil {
		return err
	}
	groups, _ := prices["items"].([]any)
	if len(groups) != 1 {
		return errors.New("community station price missing")
	}
	g, _ := groups[0].(map[string]any)
	community, _ := g["community"].(map[string]any)
	if community["amount_milli_brl"] != float64(5899) || g["official"] != nil {
		return errors.New("source separation failed")
	}
	summary, err := public("/v1/community/summary?state=MT&municipality_code=5107925")
	if err != nil {
		return err
	}
	items, _ := summary["items"].([]any)
	found := false
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		count, _ := item["station_count"].(float64)
		if item["fuel_product"] == "ETHANOL" && item["source"] == "COMMUNITY" && item["unit"] == "L" && count >= 1 {
			found = true
		}
	}
	if !found {
		return errors.New("municipal source summary missing")
	}
	p.checks = append(p.checks, "two-fuel-community-projection-station-and-municipal-summary")
	for index, choice := range []string{"HELPFUL", "NOT_HELPFUL"} {
		if _, err := call(b, "POST", "/v1/observations/"+observation+"/helpfulness", map[string]any{"client_submission_id": fmt.Sprintf("staging-community-vote-%d", index), "decision_sequence": sequence, "choice": choice}, 201); err != nil {
			return err
		}
		tally, err := call(b, "GET", fmt.Sprintf("/v1/observations/%s/helpfulness?decision_sequence=%.0f", observation, sequence), nil, 200)
		if err != nil {
			return err
		}
		if tally["my_choice"] != choice || tally["total"] != float64(1) {
			return errors.New("vote flip not consolidated")
		}
	}
	removed, err := call(b, "POST", "/v1/observations/"+observation+"/helpfulness/remove", map[string]any{"decision_sequence": sequence}, 200)
	if err != nil {
		return err
	}
	if removed["total"] != float64(0) {
		return errors.New("vote removal not consolidated")
	}
	if _, err := call(b, "POST", "/v1/observations/"+observation+"/disputes", map[string]any{"client_submission_id": "staging-community-report", "reason": "WRONG_PRODUCT"}, 201); err != nil {
		return err
	}
	reviewed, err := public(fmt.Sprintf("/v1/observations/%s/helpfulness?decision_sequence=%.0f", observation, sequence))
	if err != nil {
		return err
	}
	if reviewed["under_review"] != true || reviewed["my_choice"] != nil {
		return errors.New("report queue annotation/privacy failed")
	}
	p.checks = append(p.checks, "signed-like-dislike-remove-report-and-anonymous-review")
	return nil
}
