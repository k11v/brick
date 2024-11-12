package oper

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/k11v/brick/internal/app/build"
)

// TODO: Add t.Parallel().
func TestServiceCreateBuild(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		BuildsAllowed: 10,
	}

	defaultCreateBuildResult := &build.Build{
		// Done:             false,
		// Error:            nil,
		ID: uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
		// NextContextToken: "",
		OutputFile: nil,
	}
	defaultGetBuildByIdempotencyKeyFunc := func() (*build.Build, error) {
		return &build.Build{
			// Done:             false,
			// Error:            nil,
			ID: uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
			// NextContextToken: "",
			OutputFile: nil,
		}, nil
	}
	defaultCreateBuildParams := &CreateBuildParams{
		ContextToken:   "",
		DocumentFiles:  make(map[string][]byte),
		IdempotencyKey: uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000"),
		UserID:         uuid.MustParse("cccccccc-0000-0000-0000-000000000000"),
	}
	defaultWant := &build.Build{
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
		want               *build.Build
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
