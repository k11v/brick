package build

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type FakeDatabaseBuild struct {
	DatabaseBuild  DatabaseBuild
	ContextToken   string
	DocumentFiles  map[string][]byte
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
	CreatedAt      time.Time
}

type FakeDatabase struct {
	builds       []*FakeDatabaseBuild
	mu           sync.Mutex
	muFromUserID map[uuid.UUID]*sync.Mutex
}

func (d *FakeDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	b := &FakeDatabaseBuild{
		DatabaseBuild: DatabaseBuild{
			Done:             false,
			Error:            nil,
			ID:               uuid.New(),
			NextContextToken: "",
			OutputFile:       nil,
		},
		ContextToken:   params.ContextToken,
		DocumentFiles:  params.DocumentFiles,
		IdempotencyKey: params.IdempotencyKey,
		UserID:         params.UserID,
		CreatedAt:      time.Now().UTC(),
	}
	d.builds = append(d.builds, b)
	return &b.DatabaseBuild, nil
}

func (d *FakeDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, b := range d.builds {
		if b.DatabaseBuild.ID == params.ID {
			if b.UserID != params.UserID {
				return nil, errors.New("build access denied")
			}
			return &b.DatabaseBuild, nil
		}
	}
	return nil, errors.New("build not found")
}

func (d *FakeDatabase) GetBuildCount(params *DatabaseGetBuildCountParams) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	count := 0
	for _, b := range d.builds {
		if b.CreatedAt.Equal(params.StartTime) || b.CreatedAt.After(params.StartTime) && b.CreatedAt.Before(params.EndTime) {
			if b.UserID == params.UserID {
				count++
			}
		}
	}
	return count, nil
}

func (d *FakeDatabase) ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	builds := make([]*DatabaseBuild, 0)
	pageOffset := 0
	totalSize := 0
	for _, b := range d.builds {
		if b.UserID == params.UserID {
			totalSize++
			if pageOffset < params.PageOffset {
				pageOffset++
				continue
			}
			if len(builds) >= params.PageLimit {
				continue
			}
			builds = append(builds, &b.DatabaseBuild)
		}
	}

	var nextPageOffset *int = nil
	if maybeNextPageOffset := pageOffset + len(builds); maybeNextPageOffset < totalSize {
		nextPageOffset = new(int)
		*nextPageOffset = maybeNextPageOffset
	}

	return &DatabaseListBuildsResult{
		Builds:         builds,
		NextPageOffset: nextPageOffset,
		TotalSize:      totalSize,
	}, nil
}

func (d *FakeDatabase) Begin() (tx Database, commit func() error, rollback func() error, err error) {
	d.mu.Lock()
	unlocked := false
	unlock := func() {
		if !unlocked {
			d.mu.Unlock()
			unlocked = true
		}
	}

	fakeTx := &FakeDatabase{
		// TODO: Future build-editing functions should replace builds,
		// otherwise fake transaction wouldn't be rollbackable
		// as it reuses FakeDatabaseBuilds via pointers.
		builds: append([]*FakeDatabaseBuild(nil), d.builds...),
	}
	fakeCommit := func() error {
		defer unlock()
		d.builds = fakeTx.builds
		return nil
	}
	fakeRollback := func() error {
		defer unlock()
		return nil
	}
	return fakeTx, fakeCommit, fakeRollback, nil
}

func (d *FakeDatabase) LockUser(params *DatabaseLockUserParams) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.muFromUserID == nil {
		d.muFromUserID = make(map[uuid.UUID]*sync.Mutex)
	}
	if _, ok := d.muFromUserID[params.UserID]; !ok {
		d.muFromUserID[params.UserID] = new(sync.Mutex)
	}
	d.muFromUserID[params.UserID].Lock()
	return nil
}

// TODO: Add t.Parallel().
func TestServiceCreateBuild(t *testing.T) {
	config := &Config{
		BuildsAllowed: 10,
	}

	fakeDatabase := &FakeDatabase{}

	tests := []struct {
		name              string
		createBuildParams *CreateBuildParams
		want              *Build
		wantErr           error
	}{
		{
			"creates a build",
			&CreateBuildParams{
				ContextToken:   "",
				DocumentFiles:  make(map[string][]byte),
				IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
				UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			},
			&Build{
				Done:             false,
				Error:            nil,
				ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
				NextContextToken: "",
				OutputFile:       nil,
			},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(config, fakeDatabase, 0, 0)
			got, gotErr := service.CreateBuild(tt.createBuildParams)
			want, wantErr := tt.want, tt.wantErr
			if !reflect.DeepEqual(got, want) || !errors.Is(gotErr, wantErr) {
				t.Logf("got %#v, %#v", got, gotErr)
				t.Errorf("want %#v, %#v", want, wantErr)
			}
		})
	}
}
