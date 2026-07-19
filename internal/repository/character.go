package repository

import "context"

// Character mirrors the characters table.
type Character struct {
	ID           int64
	Name         string
	LandOfOrigin string
	Stats        *string // nullable
	GuildID      *int64  // nullable (NULL for unaffiliated characters, e.g. blacksmiths)
}

type CharacterRepository interface {
	Create(ctx context.Context, db DBTX, c Character) (Character, error)
	GetByID(ctx context.Context, db DBTX, id int64) (Character, error)
	List(ctx context.Context, db DBTX) ([]Character, error)
}

type characterRepository struct{}

func NewCharacterRepository() CharacterRepository {
	return &characterRepository{}
}

func (r *characterRepository) Create(ctx context.Context, db DBTX, c Character) (Character, error) {
	err := db.QueryRow(ctx,
		`INSERT INTO characters (name, land_of_origin, stats, guild_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		c.Name, c.LandOfOrigin, c.Stats, c.GuildID,
	).Scan(&c.ID)
	if err != nil {
		return Character{}, err
	}
	return c, nil
}

func (r *characterRepository) GetByID(ctx context.Context, db DBTX, id int64) (Character, error) {
	var c Character
	err := db.QueryRow(ctx,
		`SELECT id, name, land_of_origin, stats, guild_id
		 FROM characters WHERE id = $1`,
		id,
	).Scan(&c.ID, &c.Name, &c.LandOfOrigin, &c.Stats, &c.GuildID)
	if err != nil {
		return Character{}, scanErr(err)
	}
	return c, nil
}

func (r *characterRepository) List(ctx context.Context, db DBTX) ([]Character, error) {
	rows, err := db.Query(ctx,
		`SELECT id, name, land_of_origin, stats, guild_id
		 FROM characters ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Character
	for rows.Next() {
		var c Character
		if err := rows.Scan(&c.ID, &c.Name, &c.LandOfOrigin, &c.Stats, &c.GuildID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
