package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/repository"
)

var interactionKinds = map[string]bool{
	"note": true, "call": true, "meeting": true, "email": true, "task": true,
}

// ClientInteractions returns the merged, most-recent-first timeline for a
// client: RM-entered notes/calls/tasks plus assignment history events.
func (s *RMService) ClientInteractions(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID) ([]rmdomain.Interaction, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}

	rows, err := s.interactions.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]rmdomain.Interaction, 0, len(rows)+4)
	for _, it := range rows {
		e := rmdomain.Interaction{
			ID:        it.ID,
			EventType: "interaction",
			Kind:      it.Kind,
			Body:      it.Body,
			RMName:    it.RMName,
			CreatedAt: apitime.New(it.CreatedAt),
		}
		if it.FollowUpAt != nil {
			t := apitime.New(*it.FollowUpAt)
			e.FollowUpAt = &t
		}
		if it.DoneAt != nil {
			t := apitime.New(*it.DoneAt)
			e.DoneAt = &t
		}
		out = append(out, e)
	}

	// Fold in assignment history so the log reads as one continuous story.
	ahRows, err := s.pool.Query(ctx, `
		SELECT h.action, h.reason, h.created_at,
		       COALESCE(fr.name, ''), COALESCE(tr.name, ''), COALESCE(ar.name, '')
		FROM rm_assignment_history h
		LEFT JOIN rm_users fr ON fr.id = h.from_rm_id
		LEFT JOIN rm_users tr ON tr.id = h.to_rm_id
		LEFT JOIN rm_users ar ON ar.id = h.actor_rm_id
		WHERE h.user_id = $1
		ORDER BY h.created_at DESC
	`, userID)
	if err == nil {
		defer ahRows.Close()
		for ahRows.Next() {
			var action, reason, fromName, toName, actorName string
			var createdAt time.Time
			if err := ahRows.Scan(&action, &reason, &createdAt, &fromName, &toName, &actorName); err != nil {
				continue
			}
			out = append(out, rmdomain.Interaction{
				ID:        uuid.New(),
				EventType: "assignment",
				Kind:      action,
				Body:      assignmentSentence(action, fromName, toName, actorName, reason),
				RMName:    actorName,
				CreatedAt: apitime.New(createdAt),
			})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.Time().After(out[j].CreatedAt.Time())
	})
	return out, nil
}

func assignmentSentence(action, from, to, actor, reason string) string {
	var b strings.Builder
	switch action {
	case "auto_assign":
		b.WriteString("Auto-assigned")
		if to != "" {
			b.WriteString(" to " + to)
		}
	case "assign":
		b.WriteString("Assigned")
		if to != "" {
			b.WriteString(" to " + to)
		}
	case "transfer":
		b.WriteString("Transferred")
		if from != "" {
			b.WriteString(" from " + from)
		}
		if to != "" {
			b.WriteString(" to " + to)
		}
	case "remove":
		b.WriteString("Removed from book")
		if from != "" {
			b.WriteString(" (" + from + ")")
		}
	default:
		b.WriteString(action)
	}
	if actor != "" {
		b.WriteString(" by " + actor)
	}
	if reason != "" {
		b.WriteString(" — " + reason)
	}
	return b.String()
}

// AddClientInteraction logs a note/call/meeting/task against a client.
func (s *RMService) AddClientInteraction(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID, req rmdomain.AddInteractionRequest) (*rmdomain.Interaction, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = "note"
	}
	if !interactionKinds[kind] {
		return nil, fmt.Errorf("unknown interaction kind %q: %w", kind, apiresponse.ErrValidation)
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, fmt.Errorf("interaction body is required: %w", apiresponse.ErrValidation)
	}

	in := repository.Interaction{
		UserID: userID,
		RMID:   callerRMID,
		Kind:   kind,
		Body:   body,
	}
	if req.FollowUpAt != nil && *req.FollowUpAt > 0 {
		t := time.Unix(*req.FollowUpAt, 0).UTC()
		in.FollowUpAt = &t
	}

	saved, err := s.interactions.Add(ctx, in)
	if err != nil {
		return nil, err
	}

	staff, _ := s.rmRepo.GetByID(ctx, callerRMID)
	e := &rmdomain.Interaction{
		ID:        saved.ID,
		EventType: "interaction",
		Kind:      saved.Kind,
		Body:      saved.Body,
		CreatedAt: apitime.New(saved.CreatedAt),
	}
	if staff != nil {
		e.RMName = staff.Name
	}
	if in.FollowUpAt != nil {
		t := apitime.New(*in.FollowUpAt)
		e.FollowUpAt = &t
	}
	return e, nil
}

// CompleteInteraction marks a follow-up task done. Only the RM who logged it
// (or an admin) can close it.
func (s *RMService) CompleteInteraction(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, id uuid.UUID) error {
	err := s.interactions.Complete(ctx, id, callerRMID)
	if err == nil {
		return nil
	}
	if err == pgx.ErrNoRows && isAdmin {
		// Admins may close any staff member's follow-up.
		ct, e := s.pool.Exec(ctx,
			`UPDATE rm_client_interactions SET done_at = now() WHERE id = $1 AND done_at IS NULL`, id)
		if e != nil {
			return e
		}
		if ct.RowsAffected() == 0 {
			return apiresponse.NotFound("follow-up %s not found", id)
		}
		return nil
	}
	if err == pgx.ErrNoRows {
		return apiresponse.NotFound("follow-up %s not found", id)
	}
	return err
}

// PendingFollowUps lists the RM's open follow-up tasks for the dashboard.
func (s *RMService) PendingFollowUps(ctx context.Context, rmID uuid.UUID) ([]rmdomain.PendingFollowUp, error) {
	rows, err := s.interactions.PendingFollowUps(ctx, rmID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]rmdomain.PendingFollowUp, 0, len(rows))
	for _, p := range rows {
		fu := time.Time{}
		if p.FollowUpAt != nil {
			fu = *p.FollowUpAt
		}
		out = append(out, rmdomain.PendingFollowUp{
			ID:         p.ID,
			UserID:     p.UserID,
			ClientName: p.ClientName,
			Kind:       p.Kind,
			Body:       p.Body,
			FollowUpAt: apitime.New(fu),
			Overdue:    !fu.IsZero() && fu.Before(now),
		})
	}
	return out, nil
}
