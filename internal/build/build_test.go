package build

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// TODO: Add t.Parallel().
func TestServiceCreateBuild(t *testing.T) {
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
			got, gotErr := (&Service{}).CreateBuild(tt.createBuildParams)
			want, wantErr := tt.want, tt.wantErr
			if !reflect.DeepEqual(got, want) || !errors.Is(gotErr, wantErr) {
				t.Logf("got %#v, %#v", got, gotErr)
				t.Errorf("want %#v, %#v", want, wantErr)
			}
		})
	}
}
