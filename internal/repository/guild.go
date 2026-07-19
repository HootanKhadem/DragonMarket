package repository

import "context"

// Guild mirrors the guilds table.
type Guild struct {
	ID                int64
	Name              string
	LeaderCharacterID int64
	LandOfOrigin      string
}

type GuildRepository interface {
	Create(ctx context.Context, db DBTX, g Guild) (Guild, error)
	GetByID(ctx context.Context, db DBTX, id int64) (Guild, error)
	GetByName(ctx context.Context, db DBTX, name string) (Guild, error)
	List(ctx context.Context, db DBTX) ([]Guild, error)
}

type guildRepository struct{}

func NewGuildRepository() GuildRepository {
	return &guildRepository{}
}

func (r *guildRepository) Create(ctx context.Context, db DBTX, g Guild) (Guild, error) {
	err := db.QueryRow(ctx,
		`INSERT INTO guilds (name, leader_character_id, land_of_origin)
		 VALUES ($1, $2, $3)
		 RETURNING id`,
		g.Name, g.LeaderCharacterID, g.LandOfOrigin,
	).Scan(&g.ID)
	if err != nil {
		return Guild{}, err
	}
	return g, nil
}

func (r *guildRepository) GetByID(ctx context.Context, db DBTX, id int64) (Guild, error) {
	var g Guild
	err := db.QueryRow(ctx,
		`SELECT id, name, leader_character_id, land_of_origin
		 FROM guilds WHERE id = $1`,
		id,
	).Scan(&g.ID, &g.Name, &g.LeaderCharacterID, &g.LandOfOrigin)
	if err != nil {
		return Guild{}, scanErr(err)
	}
	return g, nil
}

func (r *guildRepository) GetByName(ctx context.Context, db DBTX, name string) (Guild, error) {
	var g Guild
	err := db.QueryRow(ctx,
		`SELECT id, name, leader_character_id, land_of_origin
		 FROM guilds WHERE name = $1`,
		name,
	).Scan(&g.ID, &g.Name, &g.LeaderCharacterID, &g.LandOfOrigin)
	if err != nil {
		return Guild{}, scanErr(err)
	}
	return g, nil
}

func (r *guildRepository) List(ctx context.Context, db DBTX) ([]Guild, error) {
	rows, err := db.Query(ctx,
		`SELECT id, name, leader_character_id, land_of_origin
		 FROM guilds ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Guild
	for rows.Next() {
		var g Guild
		if err := rows.Scan(&g.ID, &g.Name, &g.LeaderCharacterID, &g.LandOfOrigin); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
