package repository

import (
	"context"
	"fmt"
	"github.com/go-pg/pg/v10"
	"github.com/go-redis/cache/v8"
	"github.com/sakuraapp/shared/model"
	"time"
)

type UserRepository struct {
	db *pg.DB
	cache *cache.Cache
}

func (u *UserRepository) GetWithDiscriminator(ctx context.Context, id model.UserId) (*model.User, error) {
	user := new(model.User)

	if err := u.cache.Once(&cache.Item{
		Ctx:   ctx,
		Key:   fmt.Sprintf("user.%d", id),
		Value: user,
		TTL:   15 * time.Minute,
		Do: func(item *cache.Item) (interface{}, error) {
			return u.FetchWithDiscriminator(id)
		},
	}); err != nil {
		return nil, err
	}

	return user, nil
}

func (u *UserRepository) FetchWithDiscriminator(id model.UserId) (*model.User, error) {
	user := new(model.User)
	err := u.db.Model(user).
		Column("user.*").
		ColumnExpr("discriminator.value AS discriminator").
		Join("LEFT JOIN discriminators AS discriminator ON discriminator.owner_id = ?", pg.Ident("user.id")).
		Where("? = ?", pg.Ident("user.id"), id).
		First()

	return user, err
}

func (u *UserRepository) GetUsersWithDiscriminators(ids []model.UserId) ([]model.User, error) {
	var users []model.User
	err := u.db.Model(&users).
		Column("user.*").
		ColumnExpr("discriminator.value AS discriminator").
		Join("LEFT JOIN discriminators AS discriminator ON discriminator.owner_id = ?", pg.Ident("user.id")).
		Where("? in (?)", pg.Ident("user.id"), pg.In(ids)).
		Select()

	return users, err
}