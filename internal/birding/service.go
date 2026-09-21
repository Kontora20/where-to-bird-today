package birding

// Store is the persistence the whole engine needs: the union of what each of
// its parts asks for. Nothing takes a Store except NewService, which hands each
// part only the methods of its own interface, so a part can be tested with a
// fake of only those.
type Store interface {
	recommendStore
	searchStore
	panelStore
	areaStore
}

// Service is the engine as one value for the router: the parts, each doing one
// thing, side by side. Their methods are promoted onto it.
type Service struct {
	*Recommender
	*SpeciesSearcher
	*SpeciesPanel
	*WeekFinder
}

// NewService builds a Service backed by store and, for drive time limits,
// driveTimes, which may be nil.
func NewService(store Store, driveTimes DriveTimeFinder) *Service {
	return &Service{
		Recommender:     newRecommender(store, driveTimes),
		SpeciesSearcher: newSpeciesSearcher(store),
		SpeciesPanel:    newSpeciesPanel(store),
		WeekFinder:      newWeekFinder(store, driveTimes),
	}
}
