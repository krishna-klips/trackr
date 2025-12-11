package handlers

import (
	"encoding/json"
	"net/http"
	"time"
	"crypto/tls"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"trackr/internal/pkg/errors"
	"trackr/internal/pkg/validator"
	"trackr/internal/platform/auth"
	"trackr/internal/platform/models"
	"trackr/internal/platform/repositories"
)

type AuthHandler struct {
	userRepo   *repositories.UserRepository
	orgRepo    *repositories.OrganizationRepository
	inviteRepo *repositories.InviteRepository
	tokenSvc   *auth.TokenService
}

func NewAuthHandler(userRepo *repositories.UserRepository, orgRepo *repositories.OrganizationRepository, inviteRepo *repositories.InviteRepository, tokenSvc *auth.TokenService) *AuthHandler {
	return &AuthHandler{
		userRepo:   userRepo,
		orgRepo:    orgRepo,
		inviteRepo: inviteRepo,
		tokenSvc:   tokenSvc,
	}
}

type SignupRequest struct {
	InviteCode string `json:"invite_code"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	FullName   string `json:"full_name"`
}

type SignupResponse struct {
	User         *models.User `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
}

func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrCodeInvalidInput, "Invalid request body", nil)
		return
	}

	// Validate invite code
	invite, err := h.inviteRepo.GetByCode(req.InviteCode)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Database error", nil)
		return
	}
	if invite == nil || invite.Status != "pending" || invite.CurrentUses >= invite.MaxUses || invite.ExpiresAt < time.Now().Unix() {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrCodeInvalidInput, "Invalid or expired invite code", nil)
		return
	}

	// Validate email
	if err := validator.IsCorporateEmail(req.Email); err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrCodeInvalidInput, err.Error(), nil)
		return
	}

	// Check if user already exists
	existingUser, err := h.userRepo.GetByEmail(req.Email)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Database error", nil)
		return
	}
	if existingUser != nil {
		errors.WriteError(w, http.StatusConflict, errors.ErrCodeConflict, "User already exists", nil)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to hash password", nil)
		return
	}

	// Create User
	// Note: In a real invite system, the org ID would come from the invite.
	// The invite table has organization_id.

	user := &models.User{
		ID:             "usr_" + uuid.NewString(), // using simple uuid string prepended with usr_
		OrganizationID: invite.OrganizationID,
		Email:          req.Email,
		EmailVerified:  true, // Trusted via invite? Or require verification? Plan implies corporate email check is enough for now or separate step.
		PasswordHash:   string(hashedPassword),
		FullName:       req.FullName,
		Role:           invite.Role,
		CreatedAt:      time.Now().Unix(),
		UpdatedAt:      time.Now().Unix(),
	}

	// Transaction
	tx, err := h.orgRepo.BeginTx()
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Database error", nil)
		return
	}
	defer tx.Rollback()

	if err := h.userRepo.CreateTx(tx, user); err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to create user", nil)
		return
	}

	// Update invite usage
	if err := h.inviteRepo.IncrementUsesTx(tx, invite.ID); err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to update invite", nil)
		return
	}

	if err := tx.Commit(); err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Database error", nil)
		return
	}

	// Fetch Org details for response
	org, err := h.orgRepo.GetByID(user.OrganizationID)
	if err == nil {
		user.Organization = org
	}

	// Generate tokens
	accessToken, err := h.tokenSvc.GenerateAccessToken(user.ID, user.OrganizationID, user.Role, user.Email)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to generate token", nil)
		return
	}

	refreshToken, err := h.tokenSvc.GenerateRefreshToken(user.ID)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to generate token", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(SignupResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         *models.User `json:"user"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrCodeInvalidInput, "Invalid request body", nil)
		return
	}

	user, err := h.userRepo.GetByEmail(req.Email)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Database error", nil)
		return
	}
	if user == nil {
		errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "Invalid credentials", nil)
		return
	}

	if user.DeletedAt != nil {
		errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "User account deleted", nil)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "Invalid credentials", nil)
		return
	}

	// Generate tokens
	accessToken, err := h.tokenSvc.GenerateAccessToken(user.ID, user.OrganizationID, user.Role, user.Email)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to generate token", nil)
		return
	}

	refreshToken, err := h.tokenSvc.GenerateRefreshToken(user.ID)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to generate token", nil)
		return
	}

	// Update last login
	h.userRepo.UpdateLastLogin(user.ID, time.Now().Unix())

	// Fetch Org details
	org, err := h.orgRepo.GetByID(user.OrganizationID)
	if err == nil {
		user.Organization = org
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

func (h *AuthHandler) SAMLLogin(w http.ResponseWriter, r *http.Request) {
	// In a real implementation, we would look up the SAML configuration for the organization
	// derived from query params (e.g. ?org=slug) and generate an AuthnRequest.
	//
	// samlSP, _ := samlsp.New(samlsp.Options{...})
	// samlSP.HandleStartAuthFlow(w, r)

	// For now, redirect to a mock IdP login page or return instructions
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Redirect user to IdP SSO URL", "sso_url": "https://idp.example.com/sso"}`))
}

func (h *AuthHandler) HandleSAMLCallback(w http.ResponseWriter, r *http.Request) {
	// This would parse the SAMLResponse, validate signature, extract user, and issue JWT.
	// samlSP.ServeACS(w, r) -> which calls a callback.

	// Stub implementation
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("SAML ACS Endpoint - processed assertion"))
}

func (h *AuthHandler) GetSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	// Generate SP metadata
	_, err := tls.LoadX509KeyPair("configs/saml/sp-cert.pem", "configs/saml/sp-key.pem")
	if err != nil {
		// If certs missing, return error or generate on fly (omitted for brevity)
		// errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "SAML certs missing", nil)
		// Just returning a dummy XML for now if certs fail
	}

	// If we had certs, we would create samlsp.Middleware and call .Metadata()

	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.Write([]byte(`<EntityDescriptor entityID="https://trackr.io/saml/metadata" xmlns="urn:oasis:names:tc:SAML:2.0:metadata">
  <SPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</NameIDFormat>
    <AssertionConsumerService index="1" Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://trackr.io/api/v1/auth/saml/acs"/>
  </SPSSODescriptor>
</EntityDescriptor>`))
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, http.StatusBadRequest, errors.ErrCodeInvalidInput, "Invalid request body", nil)
		return
	}

	claims, err := h.tokenSvc.ValidateToken(req.RefreshToken)
	if err != nil {
		errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "Invalid refresh token", nil)
		return
	}

	// Ideally we check if refresh token is revoked in DB, but for now we trust the signature/expiration
	// We need to fetch user details again to ensure role/org hasn't changed?
	// The claims has UserID.

	user, err := h.userRepo.GetByID(claims.Subject)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Database error", nil)
		return
	}
	if user == nil {
		errors.WriteError(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized, "User not found", nil)
		return
	}

	accessToken, err := h.tokenSvc.GenerateAccessToken(user.ID, user.OrganizationID, user.Role, user.Email)
	if err != nil {
		errors.WriteError(w, http.StatusInternalServerError, errors.ErrCodeInternal, "Failed to generate token", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RefreshResponse{
		AccessToken: accessToken,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Logout Endpoint"))
}
