package build

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

type StubDatabase struct {
	BuildCount     int
	BuildToCreate  *DatabaseBuild
	GetBuildFunc   func() (*DatabaseBuild, error)
	Builds         []*DatabaseBuild
	NextPageOffset *int
	TotalSize      int
}

func (d *StubDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	return d.BuildToCreate, nil
}

func (d *StubDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	return d.GetBuildFunc()
}

func (d *StubDatabase) GetBuildCount(params *DatabaseGetBuildCountParams) (int, error) {
	return d.BuildCount, nil
}

func (d *StubDatabase) ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	return &DatabaseListBuildsResult{
		Builds:         d.Builds,
		NextPageOffset: d.NextPageOffset,
		TotalSize:      d.BuildCount,
	}, nil
}

func (d *StubDatabase) Begin() (tx Database, commit func() error, rollback func() error, err error) {
	fakeTx := d
	fakeCommit := func() error {
		return nil
	}
	fakeRollback := func() error {
		return nil
	}
	return fakeTx, fakeCommit, fakeRollback, nil
}

func (d *StubDatabase) LockUser(params *DatabaseLockUserParams) error {
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
		stubDatabase      *StubDatabase
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
			&StubDatabase{
				BuildToCreate: &DatabaseBuild{
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
			service := NewService(config, tt.stubDatabase, 0, 0)
			got, gotErr := service.CreateBuild(tt.createBuildParams)
			want, wantErr := tt.want, tt.wantErr
			if !reflect.DeepEqual(got, want) || !errors.Is(gotErr, wantErr) {
				t.Logf("got %#v, %#v", got, gotErr)
				t.Errorf("want %#v, %#v", want, wantErr)
			}
		})
	}
}
