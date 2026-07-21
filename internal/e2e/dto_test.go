package e2e

import "time"

type itemDTO struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	LandOfOrigin      string `json:"land_of_origin"`
	Rarity            string `json:"rarity"`
	ForgerCharacterID int64  `json:"forger_character_id"`
	Price             int    `json:"price"`
	IsLegendary       bool   `json:"is_legendary"`
	AuctionOnly       bool   `json:"auction_only"`
}

type createItemResponseDTO struct {
	Item      itemDTO `json:"item"`
	GuildID   int64   `json:"guild_id"`
	Quantity  int     `json:"quantity"`
	ListingID *int64  `json:"listing_id,omitempty"`
}

type itemListResponseDTO struct {
	Items []itemDTO `json:"items"`
}

type purchaseResponseDTO struct {
	ItemID        int64  `json:"item_id"`
	Quantity      int    `json:"quantity"`
	UnitPrice     int    `json:"unit_price"`
	TotalPrice    int    `json:"total_price"`
	SellerGuildID int64  `json:"seller_guild_id"`
	ListingID     int64  `json:"listing_id"`
	ListingStatus string `json:"listing_status"`
}

type auctionDTO struct {
	ID           int64     `json:"id"`
	ItemID       int64     `json:"item_id"`
	OwnerGuildID int64     `json:"owner_guild_id"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	BasePrice    int       `json:"base_price"`
}

type auctionListResponseDTO struct {
	Auctions []auctionDTO `json:"auctions"`
}

type bidDTO struct {
	ID        int64     `json:"id"`
	AuctionID int64     `json:"auction_id"`
	GuildID   int64     `json:"guild_id"`
	Amount    int       `json:"amount"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type walletDTO struct {
	TotalBalance       int `json:"total_balance"`
	ReservedBalance    int `json:"reserved_balance"`
	UsableBalance      int `json:"usable_balance"`
	DailySpendingLimit int `json:"daily_spending_limit"`
	SpentToday         int `json:"spent_today"`
}

type errorDTO struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
