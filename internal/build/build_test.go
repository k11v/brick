package build

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

type MockDatabase struct {
	TransactWithUserFunc func(userID uuid.UUID) (database Database, commit func() error, rollback func() error, err error)
	GetBuildCountFunc    func(userID uuid.UUID, startTime time.Time, endTime time.Time) (int, error)
	CreateBuildFunc      func(contextToken string, documentFiles map[string][]byte, idempotencyKey uuid.UUID, userID uuid.UUID) (*Build, error)
}

func (m *MockDatabase) CreateBuild(contextToken string, documentFiles map[string][]byte, idempotencyKey uuid.UUID, userID uuid.UUID) (*Build, error) {
	return m.CreateBuildFunc(contextToken, documentFiles, idempotencyKey, userID)
}

func (m *MockDatabase) GetBuildCount(userID uuid.UUID, startTime time.Time, endTime time.Time) (int, error) {
	return m.GetBuildCountFunc(userID, startTime, endTime)
}

func (m *MockDatabase) TransactWithUser(userID uuid.UUID) (database Database, commit func() error, rollback func() error, err error) {
	return m.TransactWithUserFunc(userID)
}

// TODO: Add t.Parallel().
func TestServiceCreateBuild(t *testing.T) {
	tests := []struct {
		name              string
		createBuildParams *CreateBuildParams
		want              *Build
		wantErr           error
		config            *Config
		mockDatabase      *MockDatabase
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
			&Config{
				BuildsAllowed: 10,
			},
			&MockDatabase{
				TransactWithUserFunc: func(userID uuid.UUID) (database Database, commit func() error, rollback func() error, err error) {
					return &MockDatabase{
						GetBuildCountFunc: func(userID uuid.UUID, startTime time.Time, endTime time.Time) (int, error) {
							return 7, nil
						},
						CreateBuildFunc: func(contextToken string, documentFiles map[string][]byte, idempotencyKey uuid.UUID, userID uuid.UUID) (*Build, error) {
							return &Build{
								Done:             false,
								Error:            nil,
								ID:               uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000"),
								NextContextToken: "",
								OutputFile:       nil,
							}, nil
						},
					}, func() error { return nil }, func() error { return nil }, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(tt.config, tt.mockDatabase, 0, 0)
			got, gotErr := service.CreateBuild(tt.createBuildParams)
			want, wantErr := tt.want, tt.wantErr
			if !reflect.DeepEqual(got, want) || !errors.Is(gotErr, wantErr) {
				t.Logf("got %#v, %#v", got, gotErr)
				t.Errorf("want %#v, %#v", want, wantErr)
			}
		})
	}
}
