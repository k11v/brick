package build

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"
)

const (
	callBegin                    = "Begin"
	callCommit                   = "Commit"
	callCreateBuild              = "CreateBuild"
	callGetBuild                 = "GetBuild"
	callGetBuildByIdempotencyKey = "GetBuildByIdempotencyKey"
	callGetBuildCount            = "GetBuildCount"
	callListBuilds               = "ListBuilds"
	callLockUser                 = "LockUser"
	callRollback                 = "Rollback"
)

type SpyDatabase struct {
	GetBuildCountResult          int
	CreateBuildResult            *DatabaseBuild
	GetBuildFunc                 func() (*DatabaseBuild, error)
	GetBuildByIdempotencyKeyFunc func() (*DatabaseBuild, error)
	ListBuildsResult             *DatabaseListBuildsResult

	Calls *[]string // doesn't contain rolled back calls
}

func (d *SpyDatabase) appendCalls(c ...string) {
	if d.Calls == nil {
		d.Calls = new([]string)
	}
	*d.Calls = append(*d.Calls, c...)
}

func (d *SpyDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	d.appendCalls(callCreateBuild)
	return d.CreateBuildResult, nil
}

func (d *SpyDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	d.appendCalls(callGetBuild)
	return d.GetBuildFunc()
}

func (d *SpyDatabase) GetBuildByIdempotencyKey(params *DatabaseGetBuildByIdempotencyKeyParams) (*DatabaseBuild, error) {
	d.appendCalls(callGetBuildByIdempotencyKey)
	return d.GetBuildByIdempotencyKeyFunc()
}

func (d *SpyDatabase) GetBuildCount(params *DatabaseGetBuildCountParams) (int, error) {
	d.appendCalls(callGetBuildCount)
	return d.GetBuildCountResult, nil
}

func (d *SpyDatabase) ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	d.appendCalls(callListBuilds)
	return d.ListBuildsResult, nil
}

func (d *SpyDatabase) LockUser(params *DatabaseLockUserParams) error {
	d.appendCalls(callLockUser)
	return nil
}

func (d *SpyDatabase) BeginFunc(f func(tx Database) error) error {
	d.appendCalls(callBegin)

	tx := *d
	tx.Calls = new([]string)
	if err := f(&tx); err != nil {
		d.appendCalls(callRollback)
		return err
	}

	d.appendCalls(*tx.Calls...)
	d.appendCalls(callCommit)
	return nil
}

// TODO: Add t.Parallel().
func TestServiceCreateBuild(t *testing.T) {
	config := &Config{
		BuildsAllowed: 10,
	}

	tests := []struct {
		name               string
		createBuildParams  *CreateBuildParams
		want               *Build
		wantErr            error
		wantCallsPredicate func(calls []string) bool
		spyDatabase        *SpyDatabase
	}{
		{
			"creates a build and returns it when the user's build count is within the limit",
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
			func(calls []string) bool {
				return slices.Contains(calls, callCreateBuild)
			},
			&SpyDatabase{
				GetBuildByIdempotencyKeyFunc: func() (*DatabaseBuild, error) {
					return nil, ErrDatabaseBuildNotFound
				},
				CreateBuildResult: &DatabaseBuild{
					Done:             false,
					Error:            nil,
					ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
					NextContextToken: "",
					OutputFile:       nil,
				},
			},
		},
		{
			"doesn't create a build and returns an error when the user's build count is beyond the limit",
			&CreateBuildParams{
				ContextToken:   "",
				DocumentFiles:  make(map[string][]byte),
				IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
				UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			},
			nil,
			ErrLimitExceeded,
			func(calls []string) bool {
				return !slices.Contains(calls, callCreateBuild)
			},
			&SpyDatabase{
				GetBuildCountResult: 10,
				GetBuildByIdempotencyKeyFunc: func() (*DatabaseBuild, error) {
					return nil, ErrDatabaseBuildNotFound
				},
			},
		},
		{
			"doesn't create a build and returns the already created build when the idempotency key was used and the params match",
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
			func(calls []string) bool {
				return !slices.Contains(calls, callCreateBuild)
			},
			&SpyDatabase{
				GetBuildByIdempotencyKeyFunc: func() (*DatabaseBuild, error) {
					return &DatabaseBuild{
						Done:             false,
						Error:            nil,
						ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
						NextContextToken: "",
						OutputFile:       nil,
					}, nil
				},
			},
		},
		{
			"doesn't create a build and returns an error when the idempotency key was used and the params don't match",
			&CreateBuildParams{
				ContextToken:   "",
				DocumentFiles:  make(map[string][]byte),
				IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
				UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
			},
			nil,
			ErrIdempotencyKeyAlreadyUsed,
			func(calls []string) bool {
				return !slices.Contains(calls, callCreateBuild)
			},
			&SpyDatabase{
				GetBuildByIdempotencyKeyFunc: func() (*DatabaseBuild, error) {
					return &DatabaseBuild{
						Done:             false,
						Error:            nil,
						ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
						NextContextToken: "",
						OutputFile:       nil,
					}, nil
				},
			},
		},
		{
			"gets the user's build count and creates a build in a critical section",
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
			func(calls []string) bool {
				filtered := make([]string, 0)
				for _, c := range calls {
					switch c {
					case callBegin, callLockUser, callGetBuildCount, callCreateBuild, callCommit, callRollback:
						filtered = append(filtered, c)
					}
				}
				return reflect.DeepEqual(filtered, []string{callBegin, callLockUser, callGetBuildCount, callCreateBuild, callCommit})
			},
			&SpyDatabase{
				GetBuildByIdempotencyKeyFunc: func() (*DatabaseBuild, error) {
					return nil, ErrDatabaseBuildNotFound
				},
				CreateBuildResult: &DatabaseBuild{
					Done:             false,
					Error:            nil,
					ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
					NextContextToken: "",
					OutputFile:       nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(config, tt.spyDatabase, 0, 0)

			got, gotErr := service.CreateBuild(tt.createBuildParams)
			want, wantErr := tt.want, tt.wantErr
			if !reflect.DeepEqual(got, want) || !errors.Is(gotErr, wantErr) {
				t.Logf("got %#v, %#v", got, gotErr)
				t.Errorf("want %#v, %#v", want, wantErr)
			}

			gotCalls := *tt.spyDatabase.Calls
			if !tt.wantCallsPredicate(gotCalls) {
				t.Logf("got %v", gotCalls)
				t.Error("want it to satisfy the predicate")
			}
		})
	}
}
