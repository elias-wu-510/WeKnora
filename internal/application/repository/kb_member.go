package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListKBMembers returns active grants for a knowledge base, oldest first.
func (r *knowledgeBaseRepository) ListKBMembers(
	ctx context.Context, kbID string,
) ([]*types.KBMember, error) {
	var members []*types.KBMember
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND deleted_at IS NULL", kbID).
		Order("created_at ASC").
		Find(&members).Error
	return members, err
}

// GetKBMember returns the active grant for (kb, user), or nil when absent.
func (r *knowledgeBaseRepository) GetKBMember(
	ctx context.Context, kbID, userID string,
) (*types.KBMember, error) {
	var member types.KBMember
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND user_id = ? AND deleted_at IS NULL", kbID, userID).
		First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// UpsertKBMember creates or updates one grant. The partial unique index on
// (kb_id, user_id) WHERE deleted_at IS NULL keeps at most one live row.
func (r *knowledgeBaseRepository) UpsertKBMember(
	ctx context.Context, member *types.KBMember,
) (*types.KBMember, error) {
	existing, err := r.GetKBMember(ctx, member.KBID, member.UserID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if existing == nil {
		if member.ID == "" {
			member.ID = uuid.New().String()
		}
		member.CreatedAt, member.UpdatedAt = now, now
		if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
			return nil, err
		}
		return member, nil
	}
	existing.Role = member.Role
	existing.UpdatedAt = now
	if err := r.db.WithContext(ctx).Model(existing).
		Updates(map[string]any{"role": existing.Role, "updated_at": now}).Error; err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteKBMember soft-deletes the grant so the audit trail keeps the row.
func (r *knowledgeBaseRepository) DeleteKBMember(ctx context.Context, kbID, userID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&types.KBMember{}).
		Where("kb_id = ? AND user_id = ? AND deleted_at IS NULL", kbID, userID).
		Updates(map[string]any{"deleted_at": &now, "updated_at": now}).Error
}

// DeleteKBMembersByKB soft-deletes every grant when the KB itself is removed.
func (r *knowledgeBaseRepository) DeleteKBMembersByKB(ctx context.Context, kbID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&types.KBMember{}).
		Where("kb_id = ? AND deleted_at IS NULL", kbID).
		Updates(map[string]any{"deleted_at": &now, "updated_at": now}).Error
}
