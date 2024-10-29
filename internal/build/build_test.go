package build

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

const (
	callBegin         = "Begin"
	callCommit        = "Commit"
	callCreateBuild   = "CreateBuild"
	callGetBuild      = "GetBuild"
	callGetBuildCount = "GetBuildCount"
	callListBuilds    = "ListBuilds"
	callLockUser      = "LockUser"
	callRollback      = "Rollback"
)

type SpyDatabase struct {
	GetBuildCountResult int
	CreateBuildResult   *DatabaseBuild
	GetBuildFunc        func() (*DatabaseBuild, error)
	ListBuildsResult    *DatabaseListBuildsResult

	Calls *[]string
}

func (d *SpyDatabase) appendCall(c string) {
	if d.Calls == nil {
		d.Calls = new([]string)
	}
	*d.Calls = append(*d.Calls, c)
}

func (d *SpyDatabase) CreateBuild(params *DatabaseCreateBuildParams) (*DatabaseBuild, error) {
	d.appendCall(callCreateBuild)
	return d.CreateBuildResult, nil
}

func (d *SpyDatabase) GetBuild(params *DatabaseGetBuildParams) (*DatabaseBuild, error) {
	d.appendCall(callGetBuild)
	return d.GetBuildFunc()
}

func (d *SpyDatabase) GetBuildCount(params *DatabaseGetBuildCountParams) (int, error) {
	d.appendCall(callGetBuildCount)
	return d.GetBuildCountResult, nil
}

func (d *SpyDatabase) ListBuilds(params *DatabaseListBuildsParams) (*DatabaseListBuildsResult, error) {
	d.appendCall(callListBuilds)
	return d.ListBuildsResult, nil
}

func (d *SpyDatabase) LockUser(params *DatabaseLockUserParams) error {
	d.appendCall(callLockUser)
	return nil
}

func (d *SpyDatabase) BeginFunc(f func(tx Database) error) error {
	d.appendCall(callBegin)

	tx := *d
	if err := f(&tx); err != nil {
		d.appendCall(callRollback)
		return err
	}

	d.appendCall(callCommit)
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
				t.Logf("got %#v, %#v", want, wantErr)
				t.Errorf("want %#v, %#v", want, wantErr)
			}

			gotCalls := *tt.spyDatabase.Calls
			wantCalls := []string{callBegin, callLockUser, callGetBuildCount, callCreateBuild, callCommit}
			if !reflect.DeepEqual(gotCalls, wantCalls) {
				t.Logf("got %v", gotCalls)
				t.Errorf("want %v", wantCalls)
			}
		})
	}
}
