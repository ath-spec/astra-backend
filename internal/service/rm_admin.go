package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/repository"
)

// RMAdminService backs the admin-only console: staff (RM) lifecycle, book
// oversight, and the assign / transfer / remove / offboard operations.
type RMAdminService struct {
	rmRepo repository.RMUserRepository
	assign repository.AssignmentRepository
}

func NewRMAdminService(rmRepo repository.RMUserRepository, assign repository.AssignmentRepository) *RMAdminService {
	return &RMAdminService{rmRepo: rmRepo, assign: assign}
}

func (s *RMAdminService) CreateRM(ctx context.Context, req rmdomain.CreateRMRequest) (*rmdomain.Public, error) {
	name := strings.TrimSpace(req.Name)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if name == "" {
		return nil, apiresponse.Validation("name is required")
	}
	if !strings.Contains(email, "@") {
		return nil, apiresponse.Validation("a valid email is required")
	}
	if len(req.Password) < 8 {
		return nil, apiresponse.Validation("password must be at least 8 characters")
	}
	role := req.Role
	if role == "" {
		role = rmdomain.RoleRM
	}
	if role != rmdomain.RoleRM && role != rmdomain.RoleAdmin {
		return nil, apiresponse.Validation("role must be 'rm' or 'admin'")
	}

	if existing, err := s.rmRepo.GetByEmail(ctx, email); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, fmt.Errorf("an account with that email already exists: %w", apiresponse.ErrConflict)
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	staff, err := s.rmRepo.Create(ctx, email, hash, name, strings.TrimSpace(req.Phone), role, req.MaxPortfolios)
	if err != nil {
		return nil, err
	}
	pub := staff.Public()
	return &pub, nil
}

func (s *RMAdminService) UpdateRM(ctx context.Context, rmID uuid.UUID, req rmdomain.UpdateRMRequest) (*rmdomain.Public, error) {
	if req.Status != nil && *req.Status != rmdomain.StatusActive && *req.Status != rmdomain.StatusInactive {
		return nil, apiresponse.Validation("status must be 'active' or 'inactive'")
	}
	if req.MaxPortfolios != nil && *req.MaxPortfolios <= 0 {
		return nil, apiresponse.Validation("max_portfolios must be positive")
	}
	staff, err := s.rmRepo.Update(ctx, rmID, req)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, apiresponse.NotFound("rm %s not found", rmID)
	}
	pub := staff.Public()
	return &pub, nil
}

func (s *RMAdminService) roster(ctx context.Context) ([]rmdomain.RosterItem, error) {
	staff, err := s.rmRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.assign.CountsByRM(ctx)
	if err != nil {
		return nil, err
	}
	aum, err := s.assign.AUMByRM(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rmdomain.RosterItem, 0, len(staff))
	for _, st := range staff {
		item := rmdomain.RosterItem{
			Public:      st.Public(),
			ClientCount: counts[st.ID],
			TotalAUM:    aum[st.ID],
		}
		if st.MaxPortfolios > 0 {
			item.Utilisation = math.Round(float64(item.ClientCount)/float64(st.MaxPortfolios)*10000) / 10000
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *RMAdminService) Roster(ctx context.Context) ([]rmdomain.RosterItem, error) {
	return s.roster(ctx)
}

func (s *RMAdminService) RMDetail(ctx context.Context, rmID uuid.UUID) (*rmdomain.RMDetail, error) {
	roster, err := s.roster(ctx)
	if err != nil {
		return nil, err
	}
	var item *rmdomain.RosterItem
	for i := range roster {
		if roster[i].ID == rmID {
			item = &roster[i]
			break
		}
	}
	if item == nil {
		return nil, apiresponse.NotFound("rm %s not found", rmID)
	}
	clients, _, err := s.assign.ListClients(ctx, &rmID, nil, rmdomain.ListFilters{Limit: 200, Sort: "wealth", Order: "desc"})
	if err != nil {
		return nil, err
	}
	return &rmdomain.RMDetail{RM: *item, Clients: clients}, nil
}

// requireRM loads a staff row and errors if missing or (optionally) not an RM.
func (s *RMAdminService) requireStaff(ctx context.Context, id uuid.UUID) (*rmdomain.StaffUser, error) {
	st, err := s.rmRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, apiresponse.NotFound("rm %s not found", id)
	}
	return st, nil
}

func (s *RMAdminService) Assign(ctx context.Context, actorRMID uuid.UUID, req rmdomain.AssignRequest) error {
	if req.UserID == uuid.Nil || req.RMID == uuid.Nil {
		return apiresponse.Validation("user_id and rm_id are required")
	}
	target, err := s.requireStaff(ctx, req.RMID)
	if err != nil {
		return err
	}
	if target.Status != rmdomain.StatusActive {
		return fmt.Errorf("target RM is inactive: %w", apiresponse.ErrValidation)
	}
	owner, found, err := s.assign.OwnerOf(ctx, req.UserID)
	if err != nil {
		return err
	}
	if !found {
		return apiresponse.NotFound("client %s not found", req.UserID)
	}
	action := rmdomain.ActionAssign
	if owner != nil {
		action = rmdomain.ActionTransfer
	}
	return s.assign.ManualAssign(ctx, req.UserID, req.RMID, actorRMID, action, "")
}

func (s *RMAdminService) Transfer(ctx context.Context, actorRMID uuid.UUID, req rmdomain.TransferRequest) error {
	if req.UserID == uuid.Nil || req.ToRMID == uuid.Nil {
		return apiresponse.Validation("user_id and to_rm_id are required")
	}
	target, err := s.requireStaff(ctx, req.ToRMID)
	if err != nil {
		return err
	}
	if target.Status != rmdomain.StatusActive {
		return fmt.Errorf("target RM is inactive: %w", apiresponse.ErrValidation)
	}
	owner, found, err := s.assign.OwnerOf(ctx, req.UserID)
	if err != nil {
		return err
	}
	if !found {
		return apiresponse.NotFound("client %s not found", req.UserID)
	}
	action := rmdomain.ActionTransfer
	if owner == nil {
		action = rmdomain.ActionAssign
	}
	return s.assign.ManualAssign(ctx, req.UserID, req.ToRMID, actorRMID, action, req.Reason)
}

func (s *RMAdminService) Remove(ctx context.Context, actorRMID uuid.UUID, req rmdomain.RemoveRequest) error {
	if req.UserID == uuid.Nil {
		return apiresponse.Validation("user_id is required")
	}
	owner, found, err := s.assign.OwnerOf(ctx, req.UserID)
	if err != nil {
		return err
	}
	if !found {
		return apiresponse.NotFound("client %s not found", req.UserID)
	}
	if owner == nil {
		return fmt.Errorf("client is already unassigned: %w", apiresponse.ErrConflict)
	}
	return s.assign.Unassign(ctx, req.UserID, actorRMID, req.Reason)
}

func (s *RMAdminService) Offboard(ctx context.Context, actorRMID, rmID uuid.UUID, reason string) (int, error) {
	if _, err := s.requireStaff(ctx, rmID); err != nil {
		return 0, err
	}
	moved, err := s.assign.OffboardRM(ctx, rmID, actorRMID, reason)
	if err != nil {
		return 0, err
	}
	return moved, nil
}

func (s *RMAdminService) Overview(ctx context.Context) (*rmdomain.AdminOverview, error) {
	staff, err := s.rmRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.assign.CountsByRM(ctx)
	if err != nil {
		return nil, err
	}
	totalClients, err := s.assign.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	unassigned, err := s.assign.CountUnassigned(ctx)
	if err != nil {
		return nil, err
	}
	totalAUM, err := s.assign.TotalAUM(ctx)
	if err != nil {
		return nil, err
	}

	ov := &rmdomain.AdminOverview{
		TotalClients:    totalClients,
		TotalAUM:        totalAUM,
		UnassignedCount: unassigned,
		RMCount:         0,
	}
	for _, st := range staff {
		if st.Role != rmdomain.RoleRM {
			continue
		}
		ov.RMCount++
		if st.Status == rmdomain.StatusActive {
			ov.ActiveRMCount++
		}
		if st.MaxPortfolios > 0 && counts[st.ID] >= st.MaxPortfolios {
			ov.RMsAtCapacity++
		}
	}
	return ov, nil
}

func (s *RMAdminService) ListClients(ctx context.Context, assigned *bool, f rmdomain.ListFilters) (*rmdomain.ClientList, error) {
	items, total, err := s.assign.ListClients(ctx, nil, assigned, f)
	if err != nil {
		return nil, err
	}
	return &rmdomain.ClientList{Items: items, Total: total}, nil
}

func (s *RMAdminService) History(ctx context.Context, userID, rmID *uuid.UUID, limit, offset int) (rmdomain.AssignmentHistory, error) {
	return s.assign.History(ctx, userID, rmID, limit, offset)
}

func (s *RMAdminService) RMClients(ctx context.Context, rmID uuid.UUID, f rmdomain.ListFilters) (*rmdomain.ClientList, error) {
	if _, err := s.requireStaff(ctx, rmID); err != nil {
		return nil, err
	}
	items, total, err := s.assign.ListClients(ctx, &rmID, nil, f)
	if err != nil {
		return nil, err
	}
	return &rmdomain.ClientList{Items: items, Total: total}, nil
}
