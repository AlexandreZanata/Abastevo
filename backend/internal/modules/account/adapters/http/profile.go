package http

import (
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

type profileProofJSON struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func (p profileProofJSON) valid() bool {
	return p.FamilyID != "" && p.AccessToken != ""
}

type emailProofJSON struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func validCode(code string) bool {
	if len(code) != domain.CodeDigits {
		return false
	}
	for i := range len(code) {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

type namedAccountJSON struct {
	AccountID  string `json:"account_id"`
	PublicName string `json:"public_name"`
}

type profileJSON struct {
	AccountID         string   `json:"account_id"`
	PublicName        string   `json:"public_name"`
	Name              *string  `json:"name"`
	ProfileComplete   bool     `json:"profile_complete"`
	RecoveryEmailHint *string  `json:"recovery_email_hint"`
	HasAccountKey     bool     `json:"has_account_key"`
	Providers         []string `json:"providers"`
}

func profileJSONOf(profile domain.AccountProfile) profileJSON {
	out := profileJSON{
		AccountID: profile.Account.ID, PublicName: profile.Account.PublicName(),
		ProfileComplete: profile.Account.DisplayName != "", HasAccountKey: profile.HasAccountKey,
		Providers: profile.Providers,
	}
	if out.ProfileComplete {
		out.Name = &profile.Account.DisplayName
	}
	if profile.RecoveryEmailHint != "" {
		out.RecoveryEmailHint = &profile.RecoveryEmailHint
	}
	return out
}

func emailLoginRetired(w http.ResponseWriter, r *http.Request) {
	fail(w, r, domain.ErrEmailLoginRetired)
}

func failProfile(w http.ResponseWriter, r *http.Request, err error) {
	// Unclassified adapter failures use the service-unavailable response
	// without exposing database details or pretending the proof was invalid.
	if domain.VerdictCode(err) == "invalid-value" {
		fail(w, r, nil)
		return
	}
	fail(w, r, err)
}

func (h Handler) signUp(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		emailProofJSON
		Name string `json:"name"`
	}
	if err := read(r, &dto); err != nil || !validCode(dto.Code) {
		bad(w, r)
		return
	}
	got, err := h.Service.SignUp(r.Context(), dto.Name, dto.Email, dto.Code)
	if err != nil {
		failProfile(w, r, err)
		return
	}
	write(w, r, http.StatusCreated, map[string]any{
		"account": namedAccountJSON{AccountID: got.Account.ID, PublicName: got.Account.PublicName()},
		"session": sessionJSONOf(got.Session), "account_key": got.Key, "created": true,
	})
}

func (h Handler) recoverKey(w http.ResponseWriter, r *http.Request) {
	var dto emailProofJSON
	if err := read(r, &dto); err != nil || !validCode(dto.Code) {
		bad(w, r)
		return
	}
	got, err := h.Service.RecoverKey(r.Context(), dto.Email, dto.Code)
	if err != nil {
		failProfile(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]any{
		"account": namedAccountJSON{AccountID: got.Account.ID, PublicName: got.Account.PublicName()},
		"session": sessionJSONOf(got.Session), "account_key": got.Key, "sessions_revoked": true,
	})
}

func (h Handler) profile(w http.ResponseWriter, r *http.Request) {
	var dto profileProofJSON
	if err := read(r, &dto); err != nil || !dto.valid() {
		bad(w, r)
		return
	}
	got, err := h.Service.Profile(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		failProfile(w, r, err)
		return
	}
	write(w, r, http.StatusOK, profileJSONOf(got))
}

func (h Handler) updateName(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		profileProofJSON
		Name string `json:"name"`
	}
	if err := read(r, &dto); err != nil || !dto.valid() {
		bad(w, r)
		return
	}
	got, err := h.Service.UpdateName(r.Context(), dto.FamilyID, dto.AccessToken, dto.Name)
	if err != nil {
		failProfile(w, r, err)
		return
	}
	write(w, r, http.StatusOK, profileJSONOf(got))
}

func (h Handler) setRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		profileProofJSON
		emailProofJSON
	}
	if err := read(r, &dto); err != nil || !dto.valid() || !validCode(dto.Code) {
		bad(w, r)
		return
	}
	got, err := h.Service.SetRecoveryEmail(r.Context(), dto.FamilyID, dto.AccessToken, dto.Email, dto.Code)
	if err != nil {
		failProfile(w, r, err)
		return
	}
	write(w, r, http.StatusOK, profileJSONOf(got))
}
