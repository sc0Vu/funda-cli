package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sc0vu/funda-cli/internal/model"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) init() error {
	_, err := s.db.Exec(`
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS listing_refs (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    city TEXT,
    transaction_type TEXT,
    lastmod TEXT,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_listing_refs_city_type
ON listing_refs(city, transaction_type);

CREATE TABLE IF NOT EXISTS listings (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    transaction_type TEXT,
    city TEXT,
    address TEXT,
    price REAL,
    living_area REAL,
    bedrooms INTEGER,
    rooms INTEGER,
    energy_label TEXT,
    latitude REAL,
    longitude REAL,
    status TEXT,
    fetched_at TEXT NOT NULL,
    FOREIGN KEY(id) REFERENCES listing_refs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_listings_search
ON listings(transaction_type, city, price, living_area);
`)
	return err
}

func (s *Store) UpsertRefs(ctx context.Context, refs []model.ListingRef) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO listing_refs(id, url, city, transaction_type, lastmod, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    url=excluded.url,
    city=excluded.city,
    transaction_type=excluded.transaction_type,
    lastmod=excluded.lastmod,
    last_seen_at=excluded.last_seen_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range refs {
		if _, err := stmt.ExecContext(ctx,
			r.ID, r.URL, r.City, r.TransactionType, r.LastMod,
			r.FirstSeenAt.Format(time.RFC3339), r.LastSeenAt.Format(time.RFC3339),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertListing(ctx context.Context, l model.Listing) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO listings(id, url, transaction_type, city, address, price, living_area, bedrooms, rooms, energy_label, latitude, longitude, status, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    url=excluded.url,
    transaction_type=excluded.transaction_type,
    city=excluded.city,
    address=excluded.address,
    price=excluded.price,
    living_area=excluded.living_area,
    bedrooms=excluded.bedrooms,
    rooms=excluded.rooms,
    energy_label=excluded.energy_label,
    latitude=excluded.latitude,
    longitude=excluded.longitude,
    status=excluded.status,
    fetched_at=excluded.fetched_at`,
		l.ID, l.URL, l.TransactionType, l.City, l.Address, l.Price, l.LivingArea,
		l.Bedrooms, l.Rooms, l.EnergyLabel, l.Latitude, l.Longitude, l.Status, l.FetchedAt,
	)
	return err
}

type SearchQuery struct {
	City      string
	Category  string
	MaxPrice  float64
	MinArea   float64
	Limit     int
	Fetched   bool
	Unfetched bool
}

func (s *Store) Search(ctx context.Context, q SearchQuery) ([]model.Listing, error) {
	var where []string
	var args []any
	if q.City != "" {
		where = append(where, "LOWER(r.city) = LOWER(?)")
		args = append(args, q.City)
	}
	if q.Category != "" && q.Category != "all" {
		where = append(where, "r.transaction_type = ?")
		args = append(args, q.Category)
	}
	if q.MaxPrice > 0 {
		where = append(where, "l.price > 0 AND l.price <= ?")
		args = append(args, q.MaxPrice)
	}
	if q.MinArea > 0 {
		where = append(where, "l.living_area >= ?")
		args = append(args, q.MinArea)
	}
	if q.Fetched {
		where = append(where, "l.id IS NOT NULL")
	}
	if q.Unfetched {
		where = append(where, "l.id IS NULL")
	}

	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
SELECT r.id, r.url, r.transaction_type, r.city,
       COALESCE(l.address,''), COALESCE(l.price,0), COALESCE(l.living_area,0),
       COALESCE(l.bedrooms,0), COALESCE(l.rooms,0), COALESCE(l.energy_label,''),
       COALESCE(l.latitude,0), COALESCE(l.longitude,0), COALESCE(l.status,''),
       COALESCE(l.fetched_at,''),
       CASE WHEN l.id IS NULL THEN 0 ELSE 1 END AS has_details,
       r.last_seen_at
FROM listing_refs r
LEFT JOIN listings l ON l.id = r.id
%s
ORDER BY CASE WHEN l.price IS NULL THEN 1 ELSE 0 END, l.price ASC, r.city ASC
LIMIT ?`, clause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Listing
	for rows.Next() {
		var l model.Listing
		var hasDetails int
		var lastSeen string
		if err := rows.Scan(&l.ID, &l.URL, &l.TransactionType, &l.City, &l.Address,
			&l.Price, &l.LivingArea, &l.Bedrooms, &l.Rooms, &l.EnergyLabel,
			&l.Latitude, &l.Longitude, &l.Status, &l.FetchedAt, &hasDetails, &lastSeen); err != nil {
			return nil, err
		}
		l.HasDetails = hasDetails != 0
		l.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeen)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) GetRef(ctx context.Context, id string) (model.ListingRef, error) {
	var r model.ListingRef
	var first, last string
	err := s.db.QueryRowContext(ctx, `SELECT id,url,city,transaction_type,lastmod,first_seen_at,last_seen_at FROM listing_refs WHERE id=?`, id).
		Scan(&r.ID, &r.URL, &r.City, &r.TransactionType, &r.LastMod, &first, &last)
	if err != nil {
		return r, err
	}
	r.FirstSeenAt, _ = time.Parse(time.RFC3339, first)
	r.LastSeenAt, _ = time.Parse(time.RFC3339, last)
	return r, nil
}

func (s *Store) MissingDetails(ctx context.Context, city, category string, limit int) ([]model.ListingRef, error) {
	var where []string
	var args []any
	where = append(where, "l.id IS NULL")
	if city != "" {
		where = append(where, "LOWER(r.city)=LOWER(?)")
		args = append(args, city)
	}
	if category != "" && category != "all" {
		where = append(where, "r.transaction_type=?")
		args = append(args, category)
	}
	if limit <= 0 {
		limit = 25
	}
	args = append(args, limit)

	q := `SELECT r.id,r.url,r.city,r.transaction_type,r.lastmod,r.first_seen_at,r.last_seen_at
FROM listing_refs r LEFT JOIN listings l ON l.id=r.id
WHERE ` + strings.Join(where, " AND ") + ` ORDER BY r.last_seen_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ListingRef
	for rows.Next() {
		var r model.ListingRef
		var first, last string
		if err := rows.Scan(&r.ID, &r.URL, &r.City, &r.TransactionType, &r.LastMod, &first, &last); err != nil {
			return nil, err
		}
		r.FirstSeenAt, _ = time.Parse(time.RFC3339, first)
		r.LastSeenAt, _ = time.Parse(time.RFC3339, last)
		out = append(out, r)
	}
	return out, rows.Err()
}
