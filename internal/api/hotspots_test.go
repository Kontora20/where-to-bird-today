package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func pgtypeFloat8(value float64) pgtype.Float8 {
	return pgtype.Float8{Float64: value, Valid: true}
}

type fakeHotspotStore struct {
	createErr error
	getErr    error
	row       store.GetHotspotRow
}

func (f fakeHotspotStore) CreateHotspot(_ context.Context, params store.CreateHotspotParams) (store.CreateHotspotRow, error) {
	if f.createErr != nil {
		return store.CreateHotspotRow{}, f.createErr
	}
	return store.CreateHotspotRow{
		Name:      params.Name,
		Latitude:  pgtypeFloat8(params.Latitude),
		Longitude: pgtypeFloat8(params.Longitude),
		Region:    params.Region,
		Source:    params.Source,
		SourceRef: params.SourceRef,
	}, nil
}

func (f fakeHotspotStore) GetHotspot(_ context.Context, _ int64) (store.GetHotspotRow, error) {
	if f.getErr != nil {
		return store.GetHotspotRow{}, f.getErr
	}
	return f.row, nil
}

func TestCreateHotspot(t *testing.T) {
	t.Parallel()

	validBody := `{"name":"Nemunas Delta","latitude":55.3167,"longitude":21.3667,"region":"LT","source":"manual","source_ref":"1"}`

	tests := []struct {
		name       string
		body       string
		store      fakeHotspotStore
		wantStatus int
	}{
		{
			name:       "valid request",
			body:       validBody,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing name",
			body:       `{"latitude":55.3,"longitude":21.3,"region":"LT","source":"manual","source_ref":"1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "latitude out of range",
			body:       `{"name":"x","latitude":95,"longitude":21.3,"region":"LT","source":"manual","source_ref":"1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "longitude out of range",
			body:       `{"name":"x","latitude":55.3,"longitude":200,"region":"LT","source":"manual","source_ref":"1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "store error",
			body:       validBody,
			store:      fakeHotspotStore{createErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/hotspots", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handleCreateHotspot(testLogger(), tc.store).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}

			if tc.wantStatus == http.StatusCreated {
				var got store.CreateHotspotRow
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("body is not valid JSON: %v (body: %q)", err, rec.Body.String())
				}
				if got.Name != "Nemunas Delta" {
					t.Errorf("body.name = %q, want %q", got.Name, "Nemunas Delta")
				}
			}
		})
	}
}

func TestGetHotspot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		store      fakeHotspotStore
		wantStatus int
	}{
		{
			name:       "found",
			id:         "1",
			store:      fakeHotspotStore{row: store.GetHotspotRow{ID: 1, Name: "Nemunas Delta"}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			id:         "404",
			store:      fakeHotspotStore{getErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "non-numeric id",
			id:         "not-a-number",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "store error",
			id:         "1",
			store:      fakeHotspotStore{getErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/hotspots/"+tc.id, nil)
			req.SetPathValue("id", tc.id)
			rec := httptest.NewRecorder()

			handleGetHotspot(testLogger(), tc.store).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
