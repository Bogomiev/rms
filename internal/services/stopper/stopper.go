package stopper

import (
	"context"
	"github.com/google/uuid"
	"regexp"
	"rms/internal/domain/models"
	"rms/internal/services"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxNameLength  = 24
	maxStores      = 4
	maxPositions   = 500
	productsLimit  = 30
	dateLayout     = time.DateOnly
	maxQueryLength = 100
)

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Storage interface {
	Stoppers(context.Context) ([]models.Stopper, error)
	AddStopper(context.Context, models.StopperInput) (int64, error)
	SaveStopper(context.Context, int64, models.StopperInput) error
	DeleteStopper(context.Context, int64) error
	StopperPositions(context.Context, models.StopperPositionFilter, string) ([]models.StopperPosition, error)
	SetPositionsStopper(context.Context, []models.StopperPositionKey, *int64) error
	SetPositionsPeriod(context.Context, []models.StopperPositionKey, string, *string) error
	AddManualPosition(context.Context, models.StopperManualInput) error
	StopperProducts(context.Context, string, int) ([]models.StopperProduct, error)
	StopperTask(context.Context, int64, string, string) (models.StopperTask, error)
	SetStopperTaskCheck(context.Context, int64, models.StopperTaskCheckInput, int64) (string, error)
	StopperManager(context.Context, int64) (bool, error)
}
type Service struct {
	storage  Storage
	timezone string
}

func New(s Storage, timezone string) *Service { return &Service{storage: s, timezone: timezone} }

func validDate(value string) bool {
	_, err := time.Parse(dateLayout, value)
	return err == nil
}
func validStopper(in *models.StopperInput) bool {
	in.Name = strings.TrimSpace(in.Name)
	return in.Name != "" && utf8.RuneCountInString(in.Name) <= maxNameLength && colorPattern.MatchString(in.Color)
}
func validKeys(keys []models.StopperPositionKey) bool {
	if len(keys) == 0 || len(keys) > maxPositions {
		return false
	}
	for _, key := range keys {
		if key.ProductID == uuid.Nil || key.DocID != nil && *key.DocID == uuid.Nil {
			return false
		}
	}
	return true
}

func (s *Service) Stoppers(ctx context.Context) ([]models.Stopper, error) {
	v, e := s.storage.Stoppers(ctx)
	return v, services.PersonnelError(e)
}
func (s *Service) AddStopper(ctx context.Context, in models.StopperInput) (models.Stopper, error) {
	if !validStopper(&in) {
		return models.Stopper{}, services.ErrInvalid
	}
	id, e := s.storage.AddStopper(ctx, in)
	if e != nil {
		return models.Stopper{}, services.PersonnelError(e)
	}
	return models.Stopper{ID: id, Name: in.Name, Color: in.Color}, nil
}
func (s *Service) SaveStopper(ctx context.Context, id int64, in models.StopperInput) error {
	if id <= 0 || !validStopper(&in) {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.SaveStopper(ctx, id, in))
}
func (s *Service) DeleteStopper(ctx context.Context, id int64) error {
	if id <= 0 {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.DeleteStopper(ctx, id))
}
func (s *Service) Positions(ctx context.Context, f models.StopperPositionFilter) ([]models.StopperPosition, error) {
	if !validDate(f.From) || !validDate(f.To) || f.From > f.To || len(f.Stores) > maxStores {
		return nil, services.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range f.Stores {
		if id == uuid.Nil || seen[id] {
			return nil, services.ErrInvalid
		}
		seen[id] = true
	}
	v, e := s.storage.StopperPositions(ctx, f, s.timezone)
	return v, services.PersonnelError(e)
}
func (s *Service) SetPositionsStopper(ctx context.Context, in models.StopperAssignInput) error {
	if !validKeys(in.Positions) || in.StopperID != nil && *in.StopperID <= 0 {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.SetPositionsStopper(ctx, in.Positions, in.StopperID))
}
func (s *Service) SetPositionsPeriod(ctx context.Context, in models.StopperPeriodInput) error {
	if !validKeys(in.Positions) || !validDate(in.From) || in.To != nil && (!validDate(*in.To) || *in.To < in.From) {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.SetPositionsPeriod(ctx, in.Positions, in.From, in.To))
}
func (s *Service) AddManualPosition(ctx context.Context, in models.StopperManualInput) error {
	if in.ProductID == uuid.Nil || in.StopperID <= 0 || !validDate(in.From) {
		return services.ErrInvalid
	}
	return services.PersonnelError(s.storage.AddManualPosition(ctx, in))
}

func (s *Service) Products(ctx context.Context, query string) ([]models.StopperProduct, error) {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > maxQueryLength {
		return nil, services.ErrInvalid
	}
	v, e := s.storage.StopperProducts(ctx, query, productsLimit)
	return v, services.PersonnelError(e)
}

// Task returns the store screen of a «Стопперы» task for today in the scheduler timezone.
func (s *Service) Task(ctx context.Context, id int64, now time.Time) (models.StopperTask, error) {
	if id <= 0 {
		return models.StopperTask{}, services.ErrInvalid
	}
	location, err := time.LoadLocation(s.timezone)
	if err != nil {
		return models.StopperTask{}, err
	}
	v, e := s.storage.StopperTask(ctx, id, now.In(location).Format(dateLayout), s.timezone)
	return v, services.PersonnelError(e)
}

var checkResults = map[string]bool{"": true, "ok": true, "bad": true, "removed": true}

// SetTaskCheck saves a store result for a task position and returns the task status.
func (s *Service) SetTaskCheck(ctx context.Context, id int64, in models.StopperTaskCheckInput, user int64) (string, error) {
	if id <= 0 || user <= 0 || in.ProductID == uuid.Nil || in.DocID != nil && *in.DocID == uuid.Nil || !checkResults[in.Result] {
		return "", services.ErrInvalid
	}
	v, e := s.storage.SetStopperTaskCheck(ctx, id, in, user)
	return v, services.PersonnelError(e)
}

// CanManage reports whether the user may open and change stopper management (admin or manager role).
func (s *Service) CanManage(ctx context.Context, user int64) (bool, error) {
	if user <= 0 {
		return false, nil
	}
	return s.storage.StopperManager(ctx, user)
}
