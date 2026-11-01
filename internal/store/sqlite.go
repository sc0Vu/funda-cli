package store

import (
	"context"
	"database/sql"
	"errors"
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
	if err != nil {
		return err
	}
	// A sitemap lastmod is a modification hint, not a publication date.
	// Record the version successfully enriched so changes can trigger refresh.
	has, err := s.columnExists("listings", "enriched_lastmod")
	if err != nil {
		return err
	}
	if !has {
		if _, err := s.db.Exec(`ALTER TABLE listings ADD COLUMN enriched_lastmod TEXT`); err != nil {
			return err
		}
	}
	if err := s.initLifecycle(); err != nil {
		return err
	}
	return nil
}

// initLifecycle is additive and keeps existing listings and delivery history intact.
func (s *Store) initLifecycle() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS listing_lifecycle (
 source TEXT NOT NULL, listing_id TEXT NOT NULL,
 availability_status TEXT NOT NULL DEFAULT 'unknown',
 last_http_status INTEGER, last_checked_at TEXT, next_check_at TEXT,
 removed_at TEXT, consecutive_not_found INTEGER NOT NULL DEFAULT 0,
 generation INTEGER NOT NULL DEFAULT 0,
 last_active_price REAL,
 PRIMARY KEY(source,listing_id),
 FOREIGN KEY(source,listing_id) REFERENCES listing_refs(source,id) ON DELETE CASCADE
 );
 CREATE INDEX IF NOT EXISTS idx_lifecycle_next_check ON listing_lifecycle(next_check_at);
 CREATE TABLE IF NOT EXISTS notification_events (
 watch_name TEXT NOT NULL, source TEXT NOT NULL, listing_id TEXT NOT NULL,
 provider TEXT NOT NULL, generation INTEGER NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending', attempts INTEGER NOT NULL DEFAULT 0,
 last_error TEXT, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, sent_at TEXT,
 PRIMARY KEY(watch_name,source,listing_id,provider,generation)
 );`)
	if err != nil {
		return err
	}
	for _, migration := range []struct{ table, col, def string }{
		{"listing_lifecycle", "last_active_price", "REAL"},
		{"notification_events", "event_type", "TEXT NOT NULL DEFAULT 'new_match'"},
		{"notification_events", "next_attempt_at", "TEXT"},
	} {
		exists, e := s.columnExists(migration.table, migration.col)
		if e != nil {
			return e
		}
		if !exists {
			if _, e = s.db.Exec("ALTER TABLE " + migration.table + " ADD COLUMN " + migration.col + " " + migration.def); e != nil {
				return e
			}
		}
	}
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS monitor_check_lock (id INTEGER PRIMARY KEY CHECK(id=1), owner TEXT NOT NULL, expires_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS watch_baseline_refs (watch_name TEXT NOT NULL, source TEXT NOT NULL, listing_id TEXT NOT NULL, PRIMARY KEY(watch_name,source,listing_id));
 CREATE TABLE IF NOT EXISTS notification_jobs (
 watch_name TEXT NOT NULL,source TEXT NOT NULL,listing_id TEXT NOT NULL,provider TEXT NOT NULL,
 generation INTEGER NOT NULL,event_type TEXT NOT NULL, event_key TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending', attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at TEXT, claimed_until TEXT, last_error TEXT, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, sent_at TEXT,
 PRIMARY KEY(watch_name,source,listing_id,provider,event_type,event_key));
 CREATE INDEX IF NOT EXISTS idx_notification_jobs_due ON notification_jobs(status,next_attempt_at);
 CREATE TABLE IF NOT EXISTS price_drop_events(source TEXT NOT NULL,listing_id TEXT NOT NULL,event_key TEXT NOT NULL,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,PRIMARY KEY(source,listing_id,event_key));`)
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

// MarkEnrichedLastmod records which sitemap version was successfully enriched.
func (s *Store) MarkEnrichedLastmod(ctx context.Context, source, id, lastmod string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE listings SET enriched_lastmod=? WHERE source=? AND id=?`, lastmod, source, id)
	return err
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
	ID        string
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
	if q.ID != "" {
		where = append(where, "r.id = ?")
		args = append(args, q.ID)
	}
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

// RefreshCandidates returns missing or stale detail records for the specified watch.
func (s *Store) RefreshCandidates(ctx context.Context, source, city, category string, olderThan time.Time, limit int) ([]model.ListingRef, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.source,r.id,r.url,r.city,r.transaction_type,r.lastmod,r.first_seen_at,r.last_seen_at
 FROM listing_refs r LEFT JOIN listings l ON l.source=r.source AND l.id=r.id
 LEFT JOIN listing_lifecycle lc ON lc.source=r.source AND lc.listing_id=r.id
 WHERE r.source=? AND LOWER(r.city)=LOWER(?) AND r.transaction_type=?
 AND ( (lc.next_check_at IS NULL OR lc.next_check_at<=?) AND (l.id IS NULL OR l.fetched_at<? OR (r.lastmod IS NOT NULL AND r.lastmod != '' AND COALESCE(l.enriched_lastmod, '') != r.lastmod)) OR (lc.next_check_at IS NOT NULL AND lc.next_check_at<=?) )
 ORDER BY CASE WHEN lc.next_check_at IS NOT NULL AND lc.next_check_at<=? THEN 0 WHEN l.id IS NULL THEN 1 WHEN (r.lastmod IS NOT NULL AND r.lastmod != '' AND COALESCE(l.enriched_lastmod, '') != r.lastmod) THEN 2 ELSE 3 END, COALESCE(lc.next_check_at,r.first_seen_at) ASC LIMIT ?`, source, city, category, time.Now().UTC().Format(time.RFC3339), olderThan.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), limit)
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
func (s *Store) InitNotifications() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS notification_deliveries (
 watch_name TEXT NOT NULL,source TEXT NOT NULL,listing_id TEXT NOT NULL,provider TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending',attempts INTEGER NOT NULL DEFAULT 0,last_error TEXT,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,sent_at TEXT,
 PRIMARY KEY(watch_name,source,listing_id,provider));`)
	return err
}

// QueueNotification atomically inserts a pending delivery if one does not exist.
func (s *Store) QueueNotification(ctx context.Context, watch, source, id, provider string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_deliveries(watch_name,source,listing_id,provider) VALUES(?,?,?,?)`, watch, source, id, provider)
	return err
}
func (s *Store) DeliveryStatus(ctx context.Context, watch, source, id, provider string) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM notification_deliveries WHERE watch_name=? AND source=? AND listing_id=? AND provider=?`, watch, source, id, provider).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return status, err
}
func (s *Store) MarkDelivery(ctx context.Context, watch, source, id, provider string, sendErr error) error {
	status, errText := "sent", ""
	if sendErr != nil {
		status = "failed"
		errText = sendErr.Error()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notification_deliveries SET status=?,attempts=attempts+1,last_error=?,sent_at=CASE WHEN ?='sent' THEN CURRENT_TIMESTAMP ELSE sent_at END WHERE watch_name=? AND source=? AND listing_id=? AND provider=?`, status, errText, status, watch, source, id, provider)
	return err
}
func (s *Store) DeliveryCount(ctx context.Context, watch string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries WHERE watch_name=?`, watch).Scan(&n)
	return n, err
}
func (s *Store) WatchInitialized(ctx context.Context, name string) (bool, error) {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_watches (name TEXT PRIMARY KEY,initialized_at TEXT NOT NULL)`); err != nil {
		return false, err
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM monitor_watches WHERE name=?`, name).Scan(&n)
	return n > 0, err
}
func (s *Store) InitializeWatch(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO monitor_watches(name,initialized_at) VALUES(?,CURRENT_TIMESTAMP)`, name)
	return err
}

// BaselineDelivery suppresses alerts for pre-existing matches on a watch's first run.
func (s *Store) BaselineDelivery(ctx context.Context, watch, source, id, provider string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_deliveries(watch_name,source,listing_id,provider,status,sent_at) VALUES(?,?,?,?,'sent',CURRENT_TIMESTAMP)`, watch, source, id, provider)
	return err
}

// RecordFetchFailure retains the last good listing data. A first 404 is only
// suspected removal; a second 404 or 410 confirms removal. 403/429/5xx do
// not imply that a home was removed.
func (s *Store) RecordFetchFailure(ctx context.Context, source, id string, code int, retry time.Duration) error {
	now := time.Now().UTC()
	if retry <= 0 {
		retry = time.Hour
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO listing_lifecycle(source,listing_id) VALUES(?,?)`, source, id)
	if err != nil {
		return err
	}
	var old string
	var misses int
	if err = tx.QueryRowContext(ctx, `SELECT availability_status,consecutive_not_found FROM listing_lifecycle WHERE source=? AND listing_id=?`, source, id).Scan(&old, &misses); err != nil {
		return err
	}
	status := old
	if code == 404 || code == 410 {
		misses++
		if code == 410 || misses >= 2 {
			status = "removed"
		} else {
			status = "suspected_removed"
		}
	}
	if code != 404 && code != 410 {
		// Access failures cannot confirm availability. Preserve the listing
		// details, but block new alerts until a successful verification.
		status = "unknown"
		misses = 0
	}
	removed := any(nil)
	if status == "removed" {
		removed = now.Format(time.RFC3339)
	}
	_, err = tx.ExecContext(ctx, `UPDATE listing_lifecycle SET availability_status=?,last_http_status=?,last_checked_at=?,next_check_at=?,removed_at=CASE WHEN ?='removed' THEN COALESCE(removed_at,?) ELSE removed_at END,consecutive_not_found=? WHERE source=? AND listing_id=?`, status, code, now.Format(time.RFC3339), now.Add(retry).Format(time.RFC3339), status, removed, misses, source, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func classifyAvailability(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "available", "beschikbaar", "te huur", "te koop":
		return "active"
	case "under_offer", "onder bod", "under_option", "verhuurd onder voorbehoud", "rented_subject_to_conditions", "sold_subject_to_conditions", "verkocht onder voorbehoud":
		return "under_option"
	case "rented", "verhuurd", "sold", "verkocht", "unavailable", "niet beschikbaar":
		return "unavailable"
	default:
		return "unknown"
	}
}

// RecordFetchSuccess detects reactivation using the same source ID. If the
// source reuses an ID after removal, a new generation permits a fresh alert.
func (s *Store) RecordFetchSuccess(ctx context.Context, source, id, rawStatus string, interval func(string) time.Duration) error {
	now := time.Now().UTC()
	status := classifyAvailability(rawStatus)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO listing_lifecycle(source,listing_id) VALUES(?,?)`, source, id)
	if err != nil {
		return err
	}
	var old string
	var gen int
	if err = tx.QueryRowContext(ctx, `SELECT availability_status,generation FROM listing_lifecycle WHERE source=? AND listing_id=?`, source, id).Scan(&old, &gen); err != nil {
		return err
	}
	if status == "active" && (old == "removed" || old == "unavailable" || old == "under_option") {
		gen++
	}
	delay := interval(status)
	if delay <= 0 {
		delay = time.Hour
	}
	_, err = tx.ExecContext(ctx, `UPDATE listing_lifecycle SET availability_status=?,last_http_status=200,last_checked_at=?,next_check_at=?,removed_at=NULL,consecutive_not_found=0,generation=? WHERE source=? AND listing_id=?`, status, now.Format(time.RFC3339), now.Add(delay).Format(time.RFC3339), gen, source, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ActiveGeneration(ctx context.Context, source, id string) (int, bool, error) {
	var status string
	var gen int
	err := s.db.QueryRowContext(ctx, `SELECT availability_status,generation FROM listing_lifecycle WHERE source=? AND listing_id=?`, source, id).Scan(&status, &gen)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return gen, status == "active", err
}
func (s *Store) QueueEvent(ctx context.Context, watch, source, id, provider string, gen int) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_events(watch_name,source,listing_id,provider,generation) VALUES(?,?,?,?,?)`, watch, source, id, provider, gen)
	return err
}
func (s *Store) EventStatus(ctx context.Context, watch, source, id, provider string, gen int) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM notification_events WHERE watch_name=? AND source=? AND listing_id=? AND provider=? AND generation=?`, watch, source, id, provider, gen).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return status, err
}
func (s *Store) MarkEvent(ctx context.Context, watch, source, id, provider string, gen int, sendErr error) error {
	status, msg := "sent", ""
	if sendErr != nil {
		status = "failed"
		msg = sendErr.Error()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notification_events SET status=?,attempts=attempts+1,last_error=?,sent_at=CASE WHEN ?='sent' THEN CURRENT_TIMESTAMP ELSE sent_at END WHERE watch_name=? AND source=? AND listing_id=? AND provider=? AND generation=?`, status, msg, status, watch, source, id, provider, gen)
	return err
}
func (s *Store) BaselineEvent(ctx context.Context, watch, source, id, provider string, gen int) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_events(watch_name,source,listing_id,provider,generation,status,sent_at) VALUES(?,?,?,?,?,'sent',CURRENT_TIMESTAMP)`, watch, source, id, provider, gen)
	return err
}

// AcquireCheckLock prevents overlapping cron/watch processes. The lease expires
// after a crash; the caller must release it on completion.
func (s *Store) AcquireCheckLock(ctx context.Context, owner string, lease time.Duration) (bool, error) {
	now := time.Now().UTC()
	until := now.Add(lease)
	res, err := s.db.ExecContext(ctx, `INSERT INTO monitor_check_lock(id,owner,expires_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET owner=excluded.owner,expires_at=excluded.expires_at WHERE expires_at<?`, owner, until.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (s *Store) ReleaseCheckLock(ctx context.Context, owner string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM monitor_check_lock WHERE id=1 AND owner=?`, owner)
	return err
}
func (s *Store) BaselineKnownRefs(ctx context.Context, watch, source, city, category string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO watch_baseline_refs(watch_name,source,listing_id)
 SELECT ?,source,id FROM listing_refs WHERE source=? AND LOWER(city)=LOWER(?) AND transaction_type=?`, watch, source, city, category)
	return err
}
func (s *Store) WasBaselineRef(ctx context.Context, watch, source, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_baseline_refs WHERE watch_name=? AND source=? AND listing_id=?`, watch, source, id).Scan(&n)
	return n > 0, err
}
func (s *Store) QueueJob(ctx context.Context, watch, source, id, provider string, gen int, eventType, eventKey string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_jobs(watch_name,source,listing_id,provider,generation,event_type,event_key) VALUES(?,?,?,?,?,?,?)`, watch, source, id, provider, gen, eventType, eventKey)
	return err
}
func (s *Store) ClaimJob(ctx context.Context, watch, source, id, provider string, gen int, eventType, eventKey string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `UPDATE notification_jobs SET status='sending',claimed_until=? WHERE watch_name=? AND source=? AND listing_id=? AND provider=? AND generation=? AND event_type=? AND event_key=? AND attempts<5 AND ((status IN ('pending','failed') AND (next_attempt_at IS NULL OR next_attempt_at<=?)) OR (status='sending' AND claimed_until<?))`, time.Now().UTC().Add(3*time.Minute).Format(time.RFC3339Nano), watch, source, id, provider, gen, eventType, eventKey, now, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (s *Store) CompleteJob(ctx context.Context, watch, source, id, provider string, gen int, eventType, eventKey string, sendErr error) error {
	status, msg := "sent", ""
	var next any
	if sendErr != nil {
		status = "failed"
		msg = sendErr.Error()
		var attempts int
		err := s.db.QueryRowContext(ctx, `SELECT attempts FROM notification_jobs WHERE watch_name=? AND source=? AND listing_id=? AND provider=? AND event_type=? AND event_key=?`, watch, source, id, provider, eventType, eventKey).Scan(&attempts)
		if err != nil {
			return err
		}
		delay := time.Minute * time.Duration(1<<min(attempts, 6))
		next = time.Now().UTC().Add(delay).Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notification_jobs SET status=?,attempts=attempts+1,last_error=?,next_attempt_at=?,claimed_until=NULL,sent_at=CASE WHEN ?='sent' THEN CURRENT_TIMESTAMP ELSE sent_at END WHERE watch_name=? AND source=? AND listing_id=? AND provider=? AND generation=? AND event_type=? AND event_key=?`, status, msg, next, status, watch, source, id, provider, gen, eventType, eventKey)
	return err
}

// CurrentPriceEvent records the most recent confirmed active price and returns
// a stable key for a newly observed price drop. Price changes are persisted
// even if the listing does not currently match a watch.
func (s *Store) CurrentPriceEvent(ctx context.Context, source, id string, price float64) (string, error) {
	if price <= 0 {
		return "", nil
	}
	var previous sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `SELECT last_active_price FROM listing_lifecycle WHERE source=? AND listing_id=?`, source, id).Scan(&previous)
	if err != nil {
		return "", err
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE listing_lifecycle SET last_active_price=? WHERE source=? AND listing_id=?`, price, source, id); err != nil {
		return "", err
	}
	if previous.Valid && price < previous.Float64 {
		return fmt.Sprintf("%.2f-to-%.2f", previous.Float64, price), nil
	}
	return "", nil
}
func (s *Store) LatestPriceDrop(ctx context.Context, source, id string) (string, error) {
	var key string
	err := s.db.QueryRowContext(ctx, `SELECT event_key FROM price_drop_events WHERE source=? AND listing_id=? ORDER BY created_at DESC LIMIT 1`, source, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return key, err
}
func (s *Store) SavePriceDrop(ctx context.Context, source, id, key string) error {
	if key == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO price_drop_events(source,listing_id,event_key) VALUES(?,?,?)`, source, id, key)
	return err
}
