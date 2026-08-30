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

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

type fakeIngestionRunStore struct {
	run       store.IngestionRun
	createErr error
	getErr    error
}

func (f fakeIngestionRunStore) CreateIngestionRun(_ context.Context, source string) (store.IngestionRun, error) {
	if f.createErr != nil {
		return store.IngestionRun{}, f.createErr
	}
	run := f.run
	run.Source = source
	return run, nil
}

func (f fakeIngestionRunStore) GetIngestionRun(_ context.Context, _ int64) (store.IngestionRun, error) {
	if f.getErr != nil {
		return store.IngestionRun{}, f.getErr
	}
	return f.run, nil
}

func TestCreateIngestionRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		store      fakeIngestionRunStore
		wantStatus int
	}{
		{
			name:       "valid request",
			body:       `{"source":"ebird"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing source",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "store error",
			body:       `{"source":"ebird"}`,
			store:      fakeIngestionRunStore{createErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/ingestion-runs", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handleCreateIngestionRun(testLogger(), tc.store).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}

			if tc.wantStatus == http.StatusCreated {
				var got store.IngestionRun
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("body is not valid JSON: %v (body: %q)", err, rec.Body.String())
				}
				if got.Source != "ebird" {
					t.Errorf("body.source = %q, want %q", got.Source, "ebird")
				}
			}
		})
	}
}

func TestGetIngestionRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		store      fakeIngestionRunStore
		wantStatus int
	}{
		{
			name:       "found",
			id:         "1",
			store:      fakeIngestionRunStore{run: store.IngestionRun{ID: 1, Source: "ebird"}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			id:         "404",
			store:      fakeIngestionRunStore{getErr: pgx.ErrNoRows},
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
			store:      fakeIngestionRunStore{getErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/ingestion-runs/"+tc.id, nil)
			req.SetPathValue("id", tc.id)
			rec := httptest.NewRecorder()

			handleGetIngestionRun(testLogger(), tc.store).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
