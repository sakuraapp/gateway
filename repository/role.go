package repository

import (
	"github.com/go-pg/pg/v10"
	"github.com/sakuraapp/shared/model"
)

type RoleRepository struct {
	db *pg.DB
}

func (r *RoleRepository) Get(userId model.UserId, roomId model.RoomId) ([]model.UserRole, error) {
	var roles []model.UserRole
	err := r.db.Model(&roles).
		Column("id", "role_id").
		Where("user_id = ?", userId).
		Where("room_id = ?", roomId).
		Order("id ASC").
		Select()

	if err == pg.ErrNoRows {
		err = nil
		roles = []model.UserRole{}
	}

	return roles, err
}

func (r *RoleRepository) Add(userRole *model.UserRole) error {
	_, err := r.db.Model(userRole).
		Column("id").
		Where("user_id = ?", userRole.UserId).
		Where("room_id = ?", userRole.RoomId).
		Where("role_id = ?", userRole.RoleId).
		OnConflict("DO NOTHING").
		Returning("id").
		SelectOrInsert()

	return err
}

func (r *RoleRepository) Remove(userRole *model.UserRole) error {
	_, err := r.db.Model(userRole).
		Where("user_id = ?", userRole.UserId).
		Where("room_id = ?", userRole.RoomId).
		Where("role_id = ?", userRole.RoleId).
		Delete()

	return err
}