package build

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

type SpyDatabase struct {
	GetBuildCountResult int
	CreateBuildResult   *DatabaseBuild
	GetBuildFunc        func() (*DatabaseBuild, error)
	ListBuildsResult    *DatabaseListBuildsResult
	Counters            struct{ Commit, Rollback int } // rollback is counted if commit wasn't called
}

func (d *SpyDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	return d.CreateBuildResult, nil
}

func (d *SpyDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	return d.GetBuildFunc()
}

func (d *SpyDatabase) GetBuildCount(params *DatabaseGetBuildCountParams) (int, error) {
	return d.GetBuildCountResult, nil
}

func (d *SpyDatabase) ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	return d.ListBuildsResult, nil
}

func (d *SpyDatabase) Begin() (tx Database, commit func() error, rollback func() error, err error) {
	committed := false

	fakeTx := d
	fakeCommit := func() error {
		committed = true
		d.Counters.Commit++
		return nil
	}
	fakeRollback := func() error {
		if committed {
			return nil
		}
		d.Counters.Rollback++
		return nil
	}
	return fakeTx, fakeCommit, fakeRollback, nil
}

func (d *SpyDatabase) LockUser(params *DatabaseLockUserParams) error {
	return nil
}

// TODO: Add t.Parallel().
func TestServiceCreateBuild(t *testing.T) {
	config := &Config{
		BuildsAllowed: 10,
	}

	tests := []struct {
		name              string
		createBuildParams *CreateBuildParams
		want              *Build
		wantErr           error
		spyDatabase       *SpyDatabase
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
			&SpyDatabase{
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

			gotCounters := tt.spyDatabase.Counters
			wantCounters := struct{ Commit, Rollback int }{1, 0}
			if !reflect.DeepEqual(gotCounters, wantCounters) {
				t.Errorf("got %#v, want %#v", gotCounters, wantCounters)
			}
		})
	}
}
