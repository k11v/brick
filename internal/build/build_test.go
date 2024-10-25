package build

import (
	"errors"
	"reflect"
	"testing"
)

// TODO: Add t.Parallel().
func TestServiceCreateBuild(t *testing.T) {
	tests := []struct {
		name string
		createBuildParams *CreateBuildParams
		want              *Build
		wantErr           error
	}{
		{"...", nil, nil, nil},
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
