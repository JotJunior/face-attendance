// Package scheduler provides the member load cycle scheduler.
// It periodically fetches members from the GOB State API and publishes
// ProcessingMessage values to RabbitMQ for each member with a selfie URL.
// Implements spec.md §FR-006 (periodic pull) + FR-007 (publish per member).
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// MemberLister fetches members from GOB State API.
type MemberLister interface {
	ListMembers(ctx context.Context) ([]domain.Member, error)
}

// MemberUpserter persists members locally.
type MemberUpserter interface {
	Upsert(ctx context.Context, m domain.Member) error
}

// ProcessingPublisher publishes a processing message per member.
type ProcessingPublisher interface {
	Publish(ctx context.Context, msg domain.ProcessingMessage) error
}

// Scheduler runs periodic member load cycles.
type Scheduler struct {
	gobClient   MemberLister
	memberRepo  MemberUpserter
	publisher   ProcessingPublisher
	logger      *logging.Logger
	intervalMin int
}

// New creates a Scheduler.
func New(
	gobClient MemberLister,
	memberRepo MemberUpserter,
	publisher ProcessingPublisher,
	logger *logging.Logger,
	intervalMinutes int,
) *Scheduler {
	return &Scheduler{
		gobClient:   gobClient,
		memberRepo:  memberRepo,
		publisher:   publisher,
		logger:      logger,
		intervalMin: intervalMinutes,
	}
}

// RunMemberLoadCycle executes one full member load cycle.
// It lists members from GOB, upserts each locally, and publishes a message
// to RabbitMQ for every member that has a selfie URL (so workers can provision them).
// This satisfies FR-006 and FR-007.
func (s *Scheduler) RunMemberLoadCycle(ctx context.Context) error {
	s.logger.Info("member_load_started", "", "", "starting member load cycle")

	members, err := s.gobClient.ListMembers(ctx)
	if err != nil {
		s.logger.Error("member_load_started", "", "", "ListMembers failed", err)
		return fmt.Errorf("scheduler: list members: %w", err)
	}

	var published, skipped, failed int
	for _, m := range members {
		if upsertErr := s.memberRepo.Upsert(ctx, m); upsertErr != nil {
			s.logger.Error("member_load_started", "", m.FederalDocument, "upsert failed", upsertErr)
			failed++
			continue
		}

		if !m.HasSelfie() {
			skipped++
			continue
		}

		msg := domain.ProcessingMessage{
			FederalDocument: m.FederalDocument,
			Name:            m.Name,
			URLSelfie:       *m.URLSelfie,
			GobID:           m.GobID,
		}
		if pubErr := s.publisher.Publish(ctx, msg); pubErr != nil {
			s.logger.Error("member_load_started", "", m.FederalDocument, "publish failed", pubErr)
			failed++
			continue
		}
		published++
	}

	s.logger.Info("member_load_started", "", "", "member load cycle complete",
		slog.Int("total", len(members)),
		slog.Int("published", published),
		slog.Int("skipped_no_selfie", skipped),
		slog.Int("failed", failed),
	)

	if failed > 0 {
		return fmt.Errorf("scheduler: %d members failed during load cycle", failed)
	}
	return nil
}

// Start runs the load cycle on a periodic interval until ctx is cancelled.
// The first run is triggered immediately, then at each intervalMin period.
func (s *Scheduler) Start(ctx context.Context) {
	s.logger.Info("member_load_started", "", "", "scheduler started",
		slog.Int("interval_minutes", s.intervalMin),
	)

	ticker := time.NewTicker(time.Duration(s.intervalMin) * time.Minute)
	defer ticker.Stop()

	// Run immediately on start
	if err := s.RunMemberLoadCycle(ctx); err != nil {
		s.logger.Error("member_load_started", "", "", "initial load cycle failed", err)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("member_load_started", "", "", "scheduler stopped")
			return
		case <-ticker.C:
			if err := s.RunMemberLoadCycle(ctx); err != nil {
				s.logger.Error("member_load_started", "", "", "periodic load cycle failed", err)
			}
		}
	}
}

// Ensure *Scheduler satisfies the httphandler.Scheduler interface.
// (This is a compile-time check — the interface is defined in internal/http.)
var _ interface {
	RunMemberLoadCycle(ctx context.Context) error
} = (*Scheduler)(nil)

// MemberRepository adapts *repository.MemberRepository to MemberUpserter.
type MemberRepository struct {
	repo *repository.MemberRepository
}

// NewMemberRepository wraps a *repository.MemberRepository for use by the scheduler.
func NewMemberRepository(r *repository.MemberRepository) *MemberRepository {
	return &MemberRepository{repo: r}
}

// Upsert delegates to the underlying repository.
func (m *MemberRepository) Upsert(ctx context.Context, member domain.Member) error {
	return m.repo.Upsert(ctx, member)
}
