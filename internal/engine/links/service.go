package links

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateLink(req *Link, customShortCode string) (*Link, error) {
	if err := ValidateLink(req); err != nil {
		return nil, err
	}

	// Generate Short Code
	shortCode, err := GenerateShortCode(customShortCode, s.repo)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	link := &Link{
		ID:               uuid.New().String(),
		ShortCode:        shortCode,
		DestinationURL:   req.DestinationURL,
		Title:            req.Title,
		CreatedBy:        req.CreatedBy,
		RedirectType:     req.RedirectType,
		Rules:            req.Rules,
		DefaultUTMParams: req.DefaultUTMParams,
		Status:           "active",
		ExpiresAt:        req.ExpiresAt,
		PasswordHash:     req.PasswordHash,
		ClickCount:       0,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if link.RedirectType == "" {
		link.RedirectType = "temporary"
	}

	if err := s.repo.Create(link); err != nil {
		return nil, err
	}

	return link, nil
}

func (s *Service) GetLink(id string) (*Link, error) {
	return s.repo.GetByID(id)
}

func (s *Service) UpdateLink(id string, updates *Link) (*Link, error) {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("link not found")
	}

	// Apply updates
	if updates.DestinationURL != "" {
		existing.DestinationURL = updates.DestinationURL
	}
	if updates.Title != "" {
		existing.Title = updates.Title
	}
	if updates.RedirectType != "" {
		existing.RedirectType = updates.RedirectType
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}
	if updates.Rules != nil {
		existing.Rules = updates.Rules
	}
	if updates.DefaultUTMParams != nil {
		existing.DefaultUTMParams = updates.DefaultUTMParams
	}
	// ... other fields

	// Validate again before saving
	if err := ValidateLink(existing); err != nil {
		return nil, err
	}

	if err := s.repo.Update(existing); err != nil {
		return nil, err
	}

	return existing, nil
}

func (s *Service) ArchiveLink(id string) error {
	return s.repo.Delete(id)
}

func (s *Service) ListLinks(limit, offset int) ([]*Link, error) {
	return s.repo.List(limit, offset)
}
