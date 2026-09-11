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
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		return err
	}
	if err := s.migrateSourceSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS listing_refs (
    source TEXT NOT NULL,
    id TEXT NOT NULL,
    url TEXT NOT NULL,
    city TEXT,
    transaction_type TEXT,
    lastmod TEXT,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    PRIMARY KEY(source, id),
    UNIQUE(source, url)
);
CREATE INDEX IF NOT EXISTS idx_listing_refs_city_type
ON listing_refs(source, city, transaction_type);

CREATE TABLE IF NOT EXISTS listings (
    source TEXT NOT NULL,
    id TEXT NOT NULL,
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
    PRIMARY KEY(source, id),
    FOREIGN KEY(source, id) REFERENCES listing_refs(source, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_listings_search
ON listings(source, transaction_type, city, price, living_area);

CREATE TABLE IF NOT EXISTS rental_details (
    source TEXT NOT NULL,
    listing_id TEXT NOT NULL,
    service_cost REAL,
    deposit REAL,
    available_from TEXT,
    income_rule TEXT,
    income_multiplier REAL,
    income_basis TEXT,
    required_income REAL,
    raw_conditions TEXT,
    PRIMARY KEY(source, listing_id),
    FOREIGN KEY(source, listing_id) REFERENCES listing_refs(source, id) ON DELETE CASCADE
);
`)
	return err
}

// migrateSourceSchema upgrades the original single-source schema in place.
// Legacy unprefixed rows become source=funda. Legacy MVGM rows that used
// mvgm:<slug> as their ID become source=mvgm, id=<slug>.
func (s *Store) migrateSourceSchema() error {
	exists, err := s.tableExists("listing_refs")
	if err != nil || !exists {
		return err
	}
	hasSource, err := s.columnExists("listing_refs", "source")
	if err != nil || hasSource {
		return err
	}

	if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF;`); err != nil {
		return err
	}
	defer s.db.Exec(`PRAGMA foreign_keys=ON;`)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`ALTER TABLE listing_refs RENAME TO listing_refs_legacy`,
		`ALTER TABLE listings RENAME TO listings_legacy`,
		`ALTER TABLE rental_details RENAME TO rental_details_legacy`,
		`CREATE TABLE listing_refs (
			source TEXT NOT NULL,
			id TEXT NOT NULL,
			url TEXT NOT NULL,
			city TEXT,
			transaction_type TEXT,
			lastmod TEXT,
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			PRIMARY KEY(source, id),
			UNIQUE(source, url)
		)`,
		`CREATE TABLE listings (
			source TEXT NOT NULL,
			id TEXT NOT NULL,
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
			PRIMARY KEY(source, id),
			FOREIGN KEY(source, id) REFERENCES listing_refs(source, id) ON DELETE CASCADE
		)`,
		`CREATE TABLE rental_details (
			source TEXT NOT NULL,
			listing_id TEXT NOT NULL,
			service_cost REAL,
			deposit REAL,
			available_from TEXT,
			income_rule TEXT,
			income_multiplier REAL,
			income_basis TEXT,
			required_income REAL,
			raw_conditions TEXT,
			PRIMARY KEY(source, listing_id),
			FOREIGN KEY(source, listing_id) REFERENCES listing_refs(source, id) ON DELETE CASCADE
		)`,
		`INSERT INTO listing_refs(source,id,url,city,transaction_type,lastmod,first_seen_at,last_seen_at)
		 SELECT CASE WHEN id LIKE 'mvgm:%' THEN 'mvgm' ELSE 'funda' END,
		        CASE WHEN id LIKE 'mvgm:%' THEN substr(id,6) ELSE id END,
		        url,city,transaction_type,lastmod,first_seen_at,last_seen_at
		 FROM listing_refs_legacy`,
		`INSERT INTO listings(source,id,url,transaction_type,city,address,price,living_area,bedrooms,rooms,energy_label,latitude,longitude,status,fetched_at)
		 SELECT CASE WHEN id LIKE 'mvgm:%' THEN 'mvgm' ELSE 'funda' END,
		        CASE WHEN id LIKE 'mvgm:%' THEN substr(id,6) ELSE id END,
		        url,transaction_type,city,address,price,living_area,bedrooms,rooms,energy_label,latitude,longitude,status,fetched_at
		 FROM listings_legacy`,
		`INSERT INTO rental_details(source,listing_id,service_cost,deposit,available_from,income_rule,income_multiplier,income_basis,required_income,raw_conditions)
		 SELECT CASE WHEN listing_id LIKE 'mvgm:%' THEN 'mvgm' ELSE 'funda' END,
		        CASE WHEN listing_id LIKE 'mvgm:%' THEN substr(listing_id,6) ELSE listing_id END,
		        service_cost,deposit,available_from,income_rule,income_multiplier,income_basis,required_income,raw_conditions
		 FROM rental_details_legacy`,
		`DROP TABLE rental_details_legacy`,
		`DROP TABLE listings_legacy`,
		`DROP TABLE listing_refs_legacy`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migrate source schema: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) tableExists(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return n > 0, err
}

func (s *Store) columnExists(table, column string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) UpsertRefs(ctx context.Context, refs []model.ListingRef) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO listing_refs(source,id,url,city,transaction_type,lastmod,first_seen_at,last_seen_at)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT(source,id) DO UPDATE SET
 url=excluded.url, city=excluded.city, transaction_type=excluded.transaction_type,
 lastmod=excluded.lastmod, last_seen_at=excluded.last_seen_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range refs {
		if r.Source == "" {
			return fmt.Errorf("listing ref %q has empty source", r.ID)
		}
		if _, err := stmt.ExecContext(ctx, r.Source, r.ID, r.URL, r.City, r.TransactionType, r.LastMod,
			r.FirstSeenAt.Format(time.RFC3339), r.LastSeenAt.Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertListing(ctx context.Context, l model.Listing) error {
	if l.Source == "" {
		return fmt.Errorf("listing %q has empty source", l.ID)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO listings(source,id,url,transaction_type,city,address,price,living_area,bedrooms,rooms,energy_label,latitude,longitude,status,fetched_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(source,id) DO UPDATE SET
 url=excluded.url, transaction_type=excluded.transaction_type, city=excluded.city,
 address=excluded.address, price=excluded.price, living_area=excluded.living_area,
 bedrooms=excluded.bedrooms, rooms=excluded.rooms, energy_label=excluded.energy_label,
 latitude=excluded.latitude, longitude=excluded.longitude, status=excluded.status, fetched_at=excluded.fetched_at`,
		l.Source, l.ID, l.URL, l.TransactionType, l.City, l.Address, l.Price, l.LivingArea, l.Bedrooms, l.Rooms, l.EnergyLabel, l.Latitude, l.Longitude, l.Status, l.FetchedAt)
	return err
}

func (s *Store) UpsertRentalDetails(ctx context.Context, d model.RentalDetails) error {
	if d.Source == "" {
		return fmt.Errorf("rental details %q has empty source", d.ListingID)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO rental_details(source,listing_id,service_cost,deposit,available_from,income_rule,income_multiplier,income_basis,required_income,raw_conditions)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(source,listing_id) DO UPDATE SET
 service_cost=excluded.service_cost, deposit=excluded.deposit, available_from=excluded.available_from,
 income_rule=excluded.income_rule, income_multiplier=excluded.income_multiplier,
 income_basis=excluded.income_basis, required_income=excluded.required_income, raw_conditions=excluded.raw_conditions`,
		d.Source, d.ListingID, d.ServiceCost, d.Deposit, d.AvailableFrom, d.IncomeRule, d.IncomeMultiplier, d.IncomeBasis, d.RequiredIncome, d.RawConditions)
	return err
}

func (s *Store) RentalDetails(ctx context.Context, source, id string) (model.RentalDetails, error) {
	var d model.RentalDetails
	err := s.db.QueryRowContext(ctx, `SELECT source,listing_id,COALESCE(service_cost,0),COALESCE(deposit,0),COALESCE(available_from,''),COALESCE(income_rule,''),COALESCE(income_multiplier,0),COALESCE(income_basis,''),COALESCE(required_income,0),COALESCE(raw_conditions,'') FROM rental_details WHERE source=? AND listing_id=?`, source, id).
		Scan(&d.Source, &d.ListingID, &d.ServiceCost, &d.Deposit, &d.AvailableFrom, &d.IncomeRule, &d.IncomeMultiplier, &d.IncomeBasis, &d.RequiredIncome, &d.RawConditions)
	return d, err
}

type SearchQuery struct {
	Source    string
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
	if q.Source != "" && q.Source != "all" {
		where = append(where, "r.source = ?")
		args = append(args, q.Source)
	}
	if q.City != "" {
		where = append(where, "LOWER(r.city)=LOWER(?)")
		args = append(args, q.City)
	}
	if q.Category != "" && q.Category != "all" {
		where = append(where, "r.transaction_type=?")
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
SELECT r.source,r.id,r.url,r.transaction_type,r.city,
 COALESCE(l.address,''),COALESCE(l.price,0),COALESCE(l.living_area,0),
 COALESCE(l.bedrooms,0),COALESCE(l.rooms,0),COALESCE(l.energy_label,''),
 COALESCE(l.latitude,0),COALESCE(l.longitude,0),COALESCE(l.status,''),COALESCE(l.fetched_at,''),
 CASE WHEN l.id IS NULL THEN 0 ELSE 1 END,r.last_seen_at
FROM listing_refs r
LEFT JOIN listings l ON l.source=r.source AND l.id=r.id
%s
ORDER BY CASE WHEN l.price IS NULL THEN 1 ELSE 0 END,l.price ASC,r.city ASC
LIMIT ?`, clause)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Listing
	for rows.Next() {
		var l model.Listing
		var has int
		var last string
		if err := rows.Scan(&l.Source, &l.ID, &l.URL, &l.TransactionType, &l.City, &l.Address, &l.Price, &l.LivingArea, &l.Bedrooms, &l.Rooms, &l.EnergyLabel, &l.Latitude, &l.Longitude, &l.Status, &l.FetchedAt, &has, &last); err != nil {
			return nil, err
		}
		l.HasDetails = has != 0
		l.LastSeenAt, _ = time.Parse(time.RFC3339, last)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) GetRef(ctx context.Context, source, id string) (model.ListingRef, error) {
	var r model.ListingRef
	var first, last string
	err := s.db.QueryRowContext(ctx, `SELECT source,id,url,city,transaction_type,lastmod,first_seen_at,last_seen_at FROM listing_refs WHERE source=? AND id=?`, source, id).
		Scan(&r.Source, &r.ID, &r.URL, &r.City, &r.TransactionType, &r.LastMod, &first, &last)
	if err != nil {
		return r, err
	}
	r.FirstSeenAt, _ = time.Parse(time.RFC3339, first)
	r.LastSeenAt, _ = time.Parse(time.RFC3339, last)
	return r, nil
}

func (s *Store) MissingDetails(ctx context.Context, source, city, category string, limit int) ([]model.ListingRef, error) {
	var where = []string{"l.id IS NULL"}
	var args []any
	if source != "" && source != "all" {
		where = append(where, "r.source=?")
		args = append(args, source)
	}
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
	q := `SELECT r.source,r.id,r.url,r.city,r.transaction_type,r.lastmod,r.first_seen_at,r.last_seen_at
FROM listing_refs r LEFT JOIN listings l ON l.source=r.source AND l.id=r.id
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
		if err := rows.Scan(&r.Source, &r.ID, &r.URL, &r.City, &r.TransactionType, &r.LastMod, &first, &last); err != nil {
			return nil, err
		}
		r.FirstSeenAt, _ = time.Parse(time.RFC3339, first)
		r.LastSeenAt, _ = time.Parse(time.RFC3339, last)
		out = append(out, r)
	}
	return out, rows.Err()
}
