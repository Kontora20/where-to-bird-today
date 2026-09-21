package api

import (
	"context"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

// Each fake implements the one interface of a handler. Their methods have
// value receivers, so a field that records an argument is a pointer: a plain
// field would be written to a copy.

// fakeRecommender is a recommender.
type fakeRecommender struct {
	err       error
	results   []birding.Recommendation
	gotParams *birding.Params
}

func (f fakeRecommender) Recommend(_ context.Context, params birding.Params) ([]birding.Recommendation, error) {
	if f.gotParams != nil {
		*f.gotParams = params
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

// fakeWeekFinder is a weekFinder.
type fakeWeekFinder struct {
	weeks         birding.AreaWeeks
	weeksErr      error
	gotAreaParams *birding.AreaParams
}

func (f fakeWeekFinder) AreaWeeks(_ context.Context, params birding.AreaParams) (birding.AreaWeeks, error) {
	if f.gotAreaParams != nil {
		*f.gotAreaParams = params
	}
	if f.weeksErr != nil {
		return birding.AreaWeeks{}, f.weeksErr
	}
	return f.weeks, nil
}

// fakeSeasonFinder is a seasonFinder.
type fakeSeasonFinder struct {
	season        birding.Season
	seasonErr     error
	gotAreaParams *birding.AreaParams
	gotLang       *string
}

func (f fakeSeasonFinder) InSeason(_ context.Context, params birding.AreaParams, lang string) (birding.Season, error) {
	if f.gotAreaParams != nil {
		*f.gotAreaParams = params
	}
	if f.gotLang != nil {
		*f.gotLang = lang
	}
	if f.seasonErr != nil {
		return birding.Season{}, f.seasonErr
	}
	return f.season, nil
}

// fakeSpeciesDetailer is a speciesDetailer.
type fakeSpeciesDetailer struct {
	speciesErr   error
	speciesFound birding.SpeciesDetail
	gotDate      *time.Time
}

func (f fakeSpeciesDetailer) SpeciesDetail(_ context.Context, _, _ int64, _ string, date time.Time) (birding.SpeciesDetail, error) {
	if f.gotDate != nil {
		*f.gotDate = date
	}
	if f.speciesErr != nil {
		return birding.SpeciesDetail{}, f.speciesErr
	}
	return f.speciesFound, nil
}

// fakeSpeciesSearcher is a speciesSearcher.
type fakeSpeciesSearcher struct {
	searchErr   error
	searchFound []birding.SpeciesMatch
}

func (f fakeSpeciesSearcher) SearchSpecies(_ context.Context, _, _ string) ([]birding.SpeciesMatch, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchFound, nil
}

// fakeRecommendationService is a recommendationService, for the tests that
// build the whole router and call none of its engine handlers.
type fakeRecommendationService struct {
	fakeRecommender
	fakeWeekFinder
	fakeSeasonFinder
	fakeSpeciesDetailer
	fakeSpeciesSearcher
}
