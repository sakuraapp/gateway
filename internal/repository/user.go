package repository

import (
	"context"
	"fmt"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/sakuraapp/shared/pkg/constant"
	model2 "github.com/sakuraapp/shared/pkg/model"
)

type UserRepository struct {
	db *pg.DB
	cache *cache.Cache
}

func (u *UserRepository) GetWithDiscriminator(ctx context.Context, id model2.UserId) (*model2.User, error) {
	user := new(model2.User)

	if err := u.cache.Once(&cache.Item{
		Ctx:   ctx,
		Key:   fmt.Sprintf(constant.UserCacheFmt, id),
		Value: user,
		TTL:   constant.UserCacheTTL,
		Do: func(item *cache.Item) (interface{}, error) {
			return u.fetchWithDiscriminator(user, id)
		},
	}); err != nil {
		return nil, err
	}

	return user, nil
}

func (u *UserRepository) fetchWithDiscriminator(user *model2.User, id model2.UserId) (*model2.User, error) {
	err := u.db.Model(user).
		Column("user.*").
		ColumnExpr("discriminator.value AS discriminator").
		Join("LEFT JOIN discriminators AS discriminator ON discriminator.owner_id = ?", pg.Ident("user.id")).
		Where("? = ?", pg.Ident("user.id"), id).
		First()

	return user, err
}

func (u *UserRepository) FetchWithDiscriminator(id model2.UserId) (*model2.User, error) {
	user := new(model2.User)

	return u.fetchWithDiscriminator(user, id)
}

func (u *UserRepository) GetUsersWithDiscriminators(ids []model2.UserId) ([]model2.User, error) {
	var users []model2.User
	err := u.db.Model(&users).
		Column("user.*").
		ColumnExpr("discriminator.value AS discriminator").
		Join("LEFT JOIN discriminators AS discriminator ON discriminator.owner_id = ?", pg.Ident("user.id")).
		Where("? in (?)", pg.Ident("user.id"), pg.In(ids)).
		Select()

	return users, err
}

func (u *UserRepository) GetRoomMembers(ids []model2.UserId, roomId model2.RoomId) ([]model2.RoomMember, error) {
	var members []model2.RoomMember
	err := u.db.Model(&members).
		Column("user.*").
		ColumnExpr("discriminator.value AS discriminator").
		Join("LEFT JOIN discriminators AS discriminator ON discriminator.owner_id = ?", pg.Ident("user.id")).
		Where("? in (?)", pg.Ident("user.id"), pg.In(ids)).
		Relation("Roles", func(q *pg.Query) (*pg.Query, error) {
			return q.Where("? = ?", pg.Ident("user_role.room_id"), roomId), nil
		}).
		Select()

	return members, err
}