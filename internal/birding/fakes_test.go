package birding

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Each fake implements the store interface of one part of the engine and
// nothing more. Their methods have value receivers, so a field that records an
// argument is a pointer: a plain field would be written to a copy.

// commonNameRows mimics the real query's DISTINCT ON + array_position ranking
// closely enough to test arg.LangPriority actually mattering: for each species,
// it picks the first language in the priority list that has a candidate,
// skipping species with no candidate in any priority language -- the same shape
// as a species with no matching row in real Postgres. candidates simulates
// species_names rows: speciesID -> lang -> name.
func commonNameRows(candidates map[int64]map[string]string, arg store.GetCommonNamesParams) []store.GetCommonNamesRow {
	var rows []store.GetCommonNamesRow
	for _, speciesID := range arg.SpeciesIds {
		for _, lang := range arg.LangPriority {
			if name, ok := candidates[speciesID][lang]; ok {
				rows = append(rows, store.GetCommonNamesRow{SpeciesID: speciesID, CommonName: name})
				break
			}
		}
	}
	return rows
}

// fakeRecommendStore is a recommendStore.
type fakeRecommendStore struct {
	recommendErr   error
	rows           []store.RecommendHotspotsRow
	recommendArgs  *store.RecommendHotspotsParams
	nameCandidates map[int64]map[string]string
	lastSeenRows   []store.HotspotSpeciesLastSeen
	lastSeenArgs   *store.GetLastSeenParams
	lastSeenErr    error
}

func (f fakeRecommendStore) RecommendHotspots(_ context.Context, arg store.RecommendHotspotsParams) ([]store.RecommendHotspotsRow, error) {
	if f.recommendArgs != nil {
		*f.recommendArgs = arg
	}
	if f.recommendErr != nil {
		return nil, f.recommendErr
	}
	return f.rows, nil
}

func (f fakeRecommendStore) GetLastSeen(_ context.Context, arg store.GetLastSeenParams) ([]store.HotspotSpeciesLastSeen, error) {
	if f.lastSeenArgs != nil {
		*f.lastSeenArgs = arg
	}
	if f.lastSeenErr != nil {
		return nil, f.lastSeenErr
	}
	return f.lastSeenRows, nil
}

func (f fakeRecommendStore) GetCommonNames(_ context.Context, arg store.GetCommonNamesParams) ([]store.GetCommonNamesRow, error) {
	return commonNameRows(f.nameCandidates, arg), nil
}

// fakeSearchStore is a searchStore.
type fakeSearchStore struct {
	searchErr  error
	searchRows []store.SearchSpeciesRow
	searchArgs *store.SearchSpeciesParams
}

func (f fakeSearchStore) SearchSpecies(_ context.Context, arg store.SearchSpeciesParams) ([]store.SearchSpeciesRow, error) {
	if f.searchArgs != nil {
		*f.searchArgs = arg
	}
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchRows, nil
}

// fakeWeekStore is a weekStore.
type fakeWeekStore struct {
	seasonalityErr  error
	seasonalityRows []store.GetHotspotSpeciesWeeksRow
}

func (f fakeWeekStore) GetHotspotSpeciesWeeks(_ context.Context, _ store.GetHotspotSpeciesWeeksParams) ([]store.GetHotspotSpeciesWeeksRow, error) {
	if f.seasonalityErr != nil {
		return nil, f.seasonalityErr
	}
	return f.seasonalityRows, nil
}

// fakeTimeOfDayStore is a timeOfDayStore. hotspotRow is the hotspot; nil means
// it does not exist.
type fakeTimeOfDayStore struct {
	hotspotRow      *store.GetHotspotRow
	timeOfDayErr    error
	timeOfDayRows   []store.GetSpeciesTimeOfDayRow
	timeOfDayParams *store.GetSpeciesTimeOfDayParams
}

func (f fakeTimeOfDayStore) GetHotspot(_ context.Context, _ int64) (store.GetHotspotRow, error) {
	if f.hotspotRow == nil {
		return store.GetHotspotRow{}, pgx.ErrNoRows
	}
	return *f.hotspotRow, nil
}

func (f fakeTimeOfDayStore) GetSpeciesTimeOfDay(_ context.Context, arg store.GetSpeciesTimeOfDayParams) ([]store.GetSpeciesTimeOfDayRow, error) {
	if f.timeOfDayParams != nil {
		*f.timeOfDayParams = arg
	}
	if f.timeOfDayErr != nil {
		return nil, f.timeOfDayErr
	}
	return f.timeOfDayRows, nil
}

// fakeAreaStore is an areaStore. hotspotsWithin, areaWeeksRows and
// speciesWeeksRows are what its queries return; areaWeeksArgs records the
// hotspots and species the weeks were asked for.
type fakeAreaStore struct {
	hotspotsWithin    []store.ListHotspotsWithinRow
	hotspotsWithinErr error
	areaWeeksRows     []store.GetAreaWeeksRow
	areaWeeksErr      error
	areaWeeksArgs     *store.GetAreaWeeksParams
	speciesWeeksRows  []store.GetAreaSpeciesWeeksRow
	speciesWeeksErr   error
	nameCandidates    map[int64]map[string]string
	mediaRows         []store.SpeciesMedium
}

func (f fakeAreaStore) ListBestSpeciesMedia(_ context.Context, _ []int64) ([]store.SpeciesMedium, error) {
	return f.mediaRows, nil
}

func (f fakeAreaStore) GetAreaSpeciesWeeks(_ context.Context, _ []int64) ([]store.GetAreaSpeciesWeeksRow, error) {
	if f.speciesWeeksErr != nil {
		return nil, f.speciesWeeksErr
	}
	return f.speciesWeeksRows, nil
}

func (f fakeAreaStore) GetCommonNames(_ context.Context, arg store.GetCommonNamesParams) ([]store.GetCommonNamesRow, error) {
	return commonNameRows(f.nameCandidates, arg), nil
}

func (f fakeAreaStore) ListHotspotsWithin(_ context.Context, _ store.ListHotspotsWithinParams) ([]store.ListHotspotsWithinRow, error) {
	if f.hotspotsWithinErr != nil {
		return nil, f.hotspotsWithinErr
	}
	return f.hotspotsWithin, nil
}

func (f fakeAreaStore) GetAreaWeeks(_ context.Context, arg store.GetAreaWeeksParams) ([]store.GetAreaWeeksRow, error) {
	if f.areaWeeksArgs != nil {
		*f.areaWeeksArgs = arg
	}
	if f.areaWeeksErr != nil {
		return nil, f.areaWeeksErr
	}
	return f.areaWeeksRows, nil
}

// fakePanelStore is a panelStore. It is the one fake that is bigger than a
// couple of methods, because the panel assembles every part of a species: it
// embeds the fakes of the parts that have their own, so their methods are
// written once. latestRow is the latest checklist for the species; nil means
// the species was never recorded at the hotspot.
type fakePanelStore struct {
	fakeWeekStore
	fakeTimeOfDayStore

	nameCandidates map[int64]map[string]string
	speciesErr     error
	speciesRow     store.Species
	mediaErr       error
	mediaRows      []store.SpeciesMedium
	latestRow      *store.GetLatestChecklistWithSpeciesRow
	latestErr      error
}

func (f fakePanelStore) GetCommonNames(_ context.Context, arg store.GetCommonNamesParams) ([]store.GetCommonNamesRow, error) {
	return commonNameRows(f.nameCandidates, arg), nil
}

func (f fakePanelStore) GetSpecies(_ context.Context, _ int64) (store.Species, error) {
	if f.speciesErr != nil {
		return store.Species{}, f.speciesErr
	}
	return f.speciesRow, nil
}

func (f fakePanelStore) ListSpeciesMedia(_ context.Context, _ int64) ([]store.SpeciesMedium, error) {
	if f.mediaErr != nil {
		return nil, f.mediaErr
	}
	return f.mediaRows, nil
}

func (f fakePanelStore) GetLatestChecklistWithSpecies(_ context.Context, _ store.GetLatestChecklistWithSpeciesParams) (store.GetLatestChecklistWithSpeciesRow, error) {
	if f.latestErr != nil {
		return store.GetLatestChecklistWithSpeciesRow{}, f.latestErr
	}
	if f.latestRow == nil {
		return store.GetLatestChecklistWithSpeciesRow{}, pgx.ErrNoRows
	}
	return *f.latestRow, nil
}
