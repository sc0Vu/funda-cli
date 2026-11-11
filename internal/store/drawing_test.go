package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sc0vu/funda-cli/internal/model"
)

func openDrawingStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedDrawingRef(t *testing.T, s *Store, id string) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.UpsertRefs(context.Background(), []model.ListingRef{{Source: "mvgm", ID: id, URL: "https://example.org/" + id, City: "Utrecht", TransactionType: "rent", FirstSeenAt: now, LastSeenAt: now}}); err != nil {
		t.Fatal(err)
	}
}

func TestRentalDetailsDrawingRoundTrip(t *testing.T) {
	s := openDrawingStore(t)
	ctx := context.Background()
	seedDrawingRef(t, s, "a")
	want := model.RentalDetails{Source: "mvgm", ListingID: "a", Deposit: 1000, HasDrawing: true, DrawingURLs: []string{"https://x/1.png", "https://x/2.png"}}
	if err := s.UpsertRentalDetails(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.RentalDetails(ctx, "mvgm", "a")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestRentalDetailsWithoutDrawing(t *testing.T) {
	s := openDrawingStore(t)
	ctx := context.Background()
	seedDrawingRef(t, s, "a")
	if err := s.UpsertRentalDetails(ctx, model.RentalDetails{Source: "mvgm", ListingID: "a"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.RentalDetails(ctx, "mvgm", "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.HasDrawing || got.DrawingURLs != nil {
		t.Fatalf("unexpected drawing: %+v", got)
	}
}

func TestRentalDetailsHasDrawingWithoutURLs(t *testing.T) {
	s := openDrawingStore(t)
	ctx := context.Background()
	seedDrawingRef(t, s, "a")
	if err := s.UpsertRentalDetails(ctx, model.RentalDetails{Source: "mvgm", ListingID: "a", HasDrawing: true}); err != nil {
		t.Fatal(err)
	}
	got, err := s.RentalDetails(ctx, "mvgm", "a")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasDrawing || got.DrawingURLs != nil {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestUpsertClearsDrawing(t *testing.T) {
	s := openDrawingStore(t)
	ctx := context.Background()
	seedDrawingRef(t, s, "a")
	if err := s.UpsertRentalDetails(ctx, model.RentalDetails{Source: "mvgm", ListingID: "a", HasDrawing: true, DrawingURLs: []string{"https://x/1.png"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRentalDetails(ctx, model.RentalDetails{Source: "mvgm", ListingID: "a"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.RentalDetails(ctx, "mvgm", "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.HasDrawing || got.DrawingURLs != nil {
		t.Fatalf("drawing not cleared: %+v", got)
	}
}

func TestMigrateRentalDetailsDrawingColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE listing_refs (source TEXT NOT NULL, id TEXT NOT NULL, url TEXT NOT NULL, city TEXT, transaction_type TEXT, lastmod TEXT, first_seen_at TEXT NOT NULL, last_seen_at TEXT NOT NULL, PRIMARY KEY(source,id), UNIQUE(source,url));
CREATE TABLE rental_details (source TEXT NOT NULL, listing_id TEXT NOT NULL, service_cost REAL, deposit REAL, available_from TEXT, income_rule TEXT, income_multiplier REAL, income_basis TEXT, required_income REAL, raw_conditions TEXT, PRIMARY KEY(source,listing_id));
INSERT INTO listing_refs VALUES ('mvgm','a','https://example.org/a','Utrecht','rent','','2024-01-01T00:00:00Z','2024-01-01T00:00:00Z');
INSERT INTO rental_details(source,listing_id,deposit) VALUES ('mvgm','a',500);`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	got, err := s.RentalDetails(ctx, "mvgm", "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Deposit != 500 || got.HasDrawing || got.DrawingURLs != nil {
		t.Fatalf("unexpected after migration: %+v", got)
	}
	got.HasDrawing, got.DrawingURLs = true, []string{"u"}
	if err := s.UpsertRentalDetails(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = s.RentalDetails(ctx, "mvgm", "a")
	if err != nil || !got.HasDrawing || len(got.DrawingURLs) != 1 {
		t.Fatalf("after upsert: %+v %v", got, err)
	}
	// reopening is idempotent
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}
