package build

import (
	"context"
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

func (d *SpyDatabase) CreateBuild(ctx context.Context, params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	d.appendCalls(callCreateBuild)
	return d.CreateBuildResult, nil
}

func (d *SpyDatabase) GetBuild(ctx context.Context, params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	d.appendCalls(callGetBuild)
	if d.GetBuildFunc == nil {
		return &DatabaseBuild{}, nil
	}
	return d.GetBuildFunc()
}

func (d *SpyDatabase) GetBuildByIdempotencyKey(ctx context.Context, params *DatabaseGetBuildByIdempotencyKeyParams) (*DatabaseBuild, error) {
	d.appendCalls(callGetBuildByIdempotencyKey)
	if d.GetBuildByIdempotencyKeyFunc == nil {
		return nil, ErrDatabaseNotFound
	}
	return d.GetBuildByIdempotencyKeyFunc()
}

func (d *SpyDatabase) GetBuildCount(ctx context.Context, params *DatabaseGetBuildCountParams) (int, error) {
	d.appendCalls(callGetBuildCount)
	return d.GetBuildCountResult, nil
}

func (d *SpyDatabase) ListBuilds(ctx context.Context, params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	d.appendCalls(callListBuilds)
	return d.ListBuildsResult, nil
}

func (d *SpyDatabase) LockUser(ctx context.Context, params *DatabaseLockUserParams) error {
	d.appendCalls(callLockUser)
	return nil
}

func (d *SpyDatabase) BeginFunc(ctx context.Context, f func(tx Database) error) error {
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
	ctx := context.Background()
	config := &Config{
		BuildsAllowed: 10,
	}

	defaultCreateBuildResult := &DatabaseBuild{
		Done:             false,
		Error:            nil,
		ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
		NextContextToken: "",
		OutputFile:       nil,
	}
	defaultGetBuildByIdempotencyKeyFunc := func() (*DatabaseBuild, error) {
		return &DatabaseBuild{
			Done:             false,
			Error:            nil,
			ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
			NextContextToken: "",
			OutputFile:       nil,
		}, nil
	}
	defaultCreateBuildParams := &CreateBuildParams{
		ContextToken:   "",
		DocumentFiles:  make(map[string][]byte),
		IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
		UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
	}
	defaultWant := &Build{
		// Done:             false,
		// Error:            nil,
		ID: uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
		// NextContextToken: "",
		OutputFile: nil,
	}

	tests := []struct {
		name               string
		spyDatabase        *SpyDatabase
		createBuildParams  *CreateBuildParams
		want               *Build
		wantErr            error
		wantCallsPredicate func(calls []string) bool
		skip               bool
	}{
		{
			name: "creates a build and returns it when the user's build count is within the limit",
			spyDatabase: &SpyDatabase{
				CreateBuildResult: defaultCreateBuildResult,
			},
			createBuildParams: defaultCreateBuildParams,
			want:              defaultWant,
			wantErr:           nil,
			wantCallsPredicate: func(calls []string) bool {
				return slices.Contains(calls, callCreateBuild)
			},
		},
		{
			name: "doesn't create a build and returns an error when the user's build count is beyond the limit",
			spyDatabase: &SpyDatabase{
				GetBuildCountResult: 10,
			},
			createBuildParams: defaultCreateBuildParams,
			want:              nil,
			wantErr:           ErrLimitExceeded,
			wantCallsPredicate: func(calls []string) bool {
				return !slices.Contains(calls, callCreateBuild)
			},
		},
		{
			name: "doesn't create a build and returns the already created build when the idempotency key was used and the params match",
			spyDatabase: &SpyDatabase{
				GetBuildByIdempotencyKeyFunc: defaultGetBuildByIdempotencyKeyFunc,
			},
			createBuildParams: defaultCreateBuildParams,
			want:              defaultWant,
			wantErr:           nil,
			wantCallsPredicate: func(calls []string) bool {
				return !slices.Contains(calls, callCreateBuild)
			},
		},
		{
			name: "doesn't create a build and returns an error when the idempotency key was used and the params don't match",
			spyDatabase: &SpyDatabase{
				GetBuildByIdempotencyKeyFunc: defaultGetBuildByIdempotencyKeyFunc,
			},
			createBuildParams: defaultCreateBuildParams,
			want:              nil,
			wantErr:           ErrIdempotencyKeyAlreadyUsed,
			wantCallsPredicate: func(calls []string) bool {
				return !slices.Contains(calls, callCreateBuild)
			},
			skip: true,
		},
		{
			name: "gets the user's build count and creates a build in a critical section",
			spyDatabase: &SpyDatabase{
				CreateBuildResult: defaultCreateBuildResult,
			},
			createBuildParams: defaultCreateBuildParams,
			want:              defaultWant,
			wantErr:           nil,
			wantCallsPredicate: func(calls []string) bool {
				filtered := make([]string, 0)
				for _, c := range calls {
					switch c {
					case callBegin, callLockUser, callGetBuildCount, callCreateBuild, callCommit, callRollback:
						filtered = append(filtered, c)
					}
				}
				return reflect.DeepEqual(filtered, []string{callBegin, callLockUser, callGetBuildCount, callCreateBuild, callCommit})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.SkipNow()
			}

			service := NewService(config, tt.spyDatabase, 0, 0)

			got, gotErr := service.CreateBuild(ctx, tt.createBuildParams)
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
